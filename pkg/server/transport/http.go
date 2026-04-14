package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/auth"
	"github.com/aidanhall34/make-mcp/pkg/config"
	rootserver "github.com/aidanhall34/make-mcp/pkg/server"
	"github.com/aidanhall34/make-mcp/pkg/telemetry"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// HTTPServer serves streamable HTTP MCP plus health endpoints.
type HTTPServer struct {
	inner      *mcpserver.StreamableHTTPServer
	server     *http.Server
	tlsCert    string
	tlsKey     string
	issuer     string
	corsOrigin string
	validator  *auth.Validator
}

// HTTPServerOption is a functional option for NewHTTPServer.
type HTTPServerOption func(*HTTPServer)

// WithValidator sets the Bearer token validator. When non-nil, the /mcp
// endpoint requires a valid Bearer token on every request.
func WithValidator(v *auth.Validator) HTTPServerOption {
	return func(s *HTTPServer) {
		s.validator = v
	}
}

// WithTLS configures the server certificate and key for HTTPS. When set,
// Start() calls ListenAndServeTLS instead of ListenAndServe.
func WithTLS(tls config.TLSConfig) HTTPServerOption {
	return func(s *HTTPServer) {
		s.tlsCert = tls.Cert
		s.tlsKey = tls.Key
	}
}

// WithOAuthIssuer sets the issuer URL advertised in the
// /.well-known/oauth-protected-resource response.
func WithOAuthIssuer(issuer string) HTTPServerOption {
	return func(s *HTTPServer) {
		s.issuer = issuer
	}
}

// WithCORSOrigin restricts which browser origins may access the /mcp endpoint.
// Provide a full origin URL (e.g. "https://claude.ai"). An empty string or "*"
// permits any origin (permissive, suitable for local development). When a
// specific origin is configured, Access-Control-Allow-Origin is only set when
// the request Origin header matches exactly; non-matching origins receive no
// CORS headers and are blocked by the browser.
func WithCORSOrigin(origin string) HTTPServerOption {
	return func(s *HTTPServer) {
		s.corsOrigin = origin
	}
}

// NewHTTPServer creates the HTTP transport wrapper.
func NewHTTPServer(toolServer *rootserver.ToolServer, addr string, opts ...HTTPServerOption) *HTTPServer {
	s := &HTTPServer{}
	for _, o := range opts {
		o(s)
	}

	inner := mcpserver.NewStreamableHTTPServer(toolServer.MCP())
	s.inner = inner

	mux := http.NewServeMux()

	// MCP endpoint — wrapped with CORS, trace propagation, and optional Bearer auth.
	mcpHandler := corsMiddleware(s.corsOrigin, propagateTraceContext(bearerMiddleware(s, inner)))
	mux.Handle("/mcp", mcpHandler)

	// Health/readiness probes — always open.
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if !toolServer.Ready() {
			http.Error(w, "not ready\n", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	// OAuth 2.0 protected resource metadata (RFC 9728 / MCP Authorization spec).
	if s.issuer != "" || s.validator != nil {
		mux.HandleFunc("/.well-known/oauth-protected-resource", s.handleOAuthMetadata)
	}

	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return s
}

// handleOAuthMetadata serves the OAuth 2.0 Protected Resource Metadata
// document required by the MCP Authorization specification.
func (s *HTTPServer) handleOAuthMetadata(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	resource := scheme + "://" + r.Host + "/"

	doc := map[string]any{
		"resource":                 resource,
		"bearer_methods_supported": []string{"header"},
	}
	if s.issuer != "" {
		doc["authorization_servers"] = []string{s.issuer}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

// bearerMiddleware wraps next with Bearer token validation when the server has
// a configured validator. Requests with missing or invalid tokens receive
// 401 Unauthorized. When no validator is configured the handler is returned
// unchanged.
func bearerMiddleware(s *HTTPServer, next http.Handler) http.Handler {
	if s.validator == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		raw := auth.BearerToken(r)
		if raw == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="make-mcp", error="invalid_request"`)
			http.Error(w, "missing Authorization Bearer token\n", http.StatusUnauthorized)
			telemetry.RecordAuthAttempt(r.Context(), telemetry.StatusFailure, time.Since(start))
			return
		}
		claims, err := s.validator.Validate(r.Context(), raw)
		elapsed := time.Since(start)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="make-mcp", error="invalid_token"`)
			http.Error(w, "invalid or expired token\n", http.StatusUnauthorized)
			telemetry.RecordAuthAttempt(r.Context(), telemetry.StatusFailure, elapsed)
			return
		}
		telemetry.RecordAuthAttempt(r.Context(), telemetry.StatusSuccess, elapsed)
		next.ServeHTTP(w, r.WithContext(auth.ContextWithClaims(r.Context(), claims)))
	})
}

// corsMiddleware adds CORS headers for browser-based MCP clients (e.g. MCP
// Inspector). It handles preflight OPTIONS requests and exposes the
// mcp-session-id response header so JavaScript can read it.
//
// allowedOrigin controls the policy:
//   - "" or "*" – wildcard; any browser origin is permitted (development default).
//   - specific URL – Access-Control-Allow-Origin is only set when the request
//     Origin header matches exactly. Non-matching origins receive no CORS headers
//     so the browser blocks the cross-origin request. Vary: Origin is added to
//     prevent incorrect cache sharing between origins.
func corsMiddleware(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")

		headersSet := false
		switch {
		case allowedOrigin == "" || allowedOrigin == "*":
			w.Header().Set("Access-Control-Allow-Origin", "*")
			headersSet = true
		case requestOrigin != "" && requestOrigin == allowedOrigin:
			w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
			w.Header().Add("Vary", "Origin")
			headersSet = true
		}

		if headersSet {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, mcp-session-id, mcp-protocol-version, Last-Event-ID")
			w.Header().Set("Access-Control-Expose-Headers", "mcp-session-id")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func propagateTraceContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := telemetry.Tracer().Start(ctx, "mcp.http.request",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", "/mcp"),
			),
		)
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Start begins serving HTTP requests. When TLS is configured (cert and key
// paths set), ListenAndServeTLS is used instead of ListenAndServe.
func (s *HTTPServer) Start() error {
	if s.tlsCert != "" && s.tlsKey != "" {
		return s.server.ListenAndServeTLS(s.tlsCert, s.tlsKey)
	}
	return s.server.ListenAndServe()
}

// StartTLS starts the server with the provided *tls.Config (useful when the
// caller needs to supply a custom certificate pool, e.g. for mTLS).
func (s *HTTPServer) StartTLS(tlsConfig *tls.Config) error {
	s.server.TLSConfig = tlsConfig
	// Empty strings cause ListenAndServeTLS to load certs from TLSConfig.
	return s.server.ListenAndServeTLS("", "")
}

// Shutdown gracefully stops the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if err := s.inner.Shutdown(ctx); err != nil {
		return err
	}
	return s.server.Shutdown(ctx)
}
