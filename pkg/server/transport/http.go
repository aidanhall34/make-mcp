package transport

import (
	"context"
	"net/http"

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
	inner  *mcpserver.StreamableHTTPServer
	server *http.Server
}

// NewHTTPServer creates the HTTP transport wrapper.
func NewHTTPServer(toolServer *rootserver.ToolServer, addr string) *HTTPServer {
	inner := mcpserver.NewStreamableHTTPServer(toolServer.MCP())
	mux := http.NewServeMux()
	mux.Handle("/mcp", propagateTraceContext(inner))
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

	return &HTTPServer{
		inner: inner,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
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

// Start begins serving HTTP requests.
func (s *HTTPServer) Start() error {
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if err := s.inner.Shutdown(ctx); err != nil {
		return err
	}
	return s.server.Shutdown(ctx)
}
