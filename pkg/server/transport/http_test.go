package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/auth"
	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	rootserver "github.com/aidanhall34/make-mcp/pkg/server"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func newToolServer(t *testing.T, recipes []parser.Recipe) *rootserver.ToolServer {
	t.Helper()
	s, err := rootserver.New(config.Default(), recipes)
	if err != nil {
		t.Fatalf("rootserver.New() error = %v", err)
	}
	return s
}

func validRecipe() parser.Recipe {
	return parser.Recipe{
		ID:          "hello",
		Name:        "Hello",
		Description: "Greets.",
		Risk:        parser.RiskLow,
		Params:      []parser.Param{},
		Output:      "greeting",
		OutputType:  "text/plain",
	}
}

func TestHTTPServer_HealthEndpoint(t *testing.T) {
	hs := NewHTTPServer(newToolServer(t, []parser.Recipe{validRecipe()}), "127.0.0.1:0")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("/health status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHTTPServer_ReadyEndpoint_Ready(t *testing.T) {
	hs := NewHTTPServer(newToolServer(t, []parser.Recipe{validRecipe()}), "127.0.0.1:0")

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("/ready status = %d, want %d (server has tools)", rr.Code, http.StatusOK)
	}
}

func TestHTTPServer_ReadyEndpoint_NotReady(t *testing.T) {
	hs := NewHTTPServer(newToolServer(t, nil), "127.0.0.1:0")

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("/ready status = %d, want %d (no tools)", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestHTTPServer_Shutdown(t *testing.T) {
	hs := NewHTTPServer(newToolServer(t, nil), "127.0.0.1:0")
	if err := hs.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestServeStdio_EOF(t *testing.T) {
	// Replace os.Stdin with a reader that immediately returns EOF so
	// ServeStdio returns without blocking.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	w.Close() // EOF on first read
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig; r.Close() })

	if err := ServeStdio(newToolServer(t, nil).MCP()); err != nil {
		t.Errorf("ServeStdio() = %v, want nil", err)
	}
}

func TestHTTPServer_Start(t *testing.T) {
	hs := NewHTTPServer(newToolServer(t, []parser.Recipe{validRecipe()}), "127.0.0.1:0")
	errCh := make(chan error, 1)
	go func() { errCh <- hs.Start() }()
	// Give it a moment to bind.
	time.Sleep(20 * time.Millisecond)
	if err := hs.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Start() = %v, want nil or ErrServerClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after Shutdown")
	}
}

func TestCORSMiddleware_Wildcard_SetsHeaders(t *testing.T) {
	for _, origin := range []string{"", "*"} {
		t.Run("allowedOrigin="+origin, func(t *testing.T) {
			handler := corsMiddleware(origin, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
			}
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
			}
			if got := rr.Header().Get("Access-Control-Expose-Headers"); got != "mcp-session-id" {
				t.Errorf("Access-Control-Expose-Headers = %q, want mcp-session-id", got)
			}
			if got := rr.Header().Get("Vary"); got != "" {
				t.Errorf("Vary should be empty for wildcard, got %q", got)
			}
		})
	}
}

func TestCORSMiddleware_Wildcard_PreflightOptions(t *testing.T) {
	handler := corsMiddleware("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("inner handler should not be called for OPTIONS preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	for _, h := range []string{"Access-Control-Allow-Methods", "Access-Control-Allow-Headers"} {
		if rr.Header().Get(h) == "" {
			t.Errorf("%s header missing on preflight response", h)
		}
	}
	allowedHeaders := rr.Header().Get("Access-Control-Allow-Headers")
	for _, want := range []string{"mcp-session-id", "mcp-protocol-version"} {
		if !strings.Contains(allowedHeaders, want) {
			t.Errorf("Access-Control-Allow-Headers missing %q, got %q", want, allowedHeaders)
		}
	}
}

func TestCORSMiddleware_SpecificOrigin_MatchingRequest(t *testing.T) {
	const allowed = "https://claude.ai"
	handler := corsMiddleware(allowed, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", allowed)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowed)
	}
	if got := rr.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary = %q, want it to contain Origin", got)
	}
	if got := rr.Header().Get("Access-Control-Expose-Headers"); got != "mcp-session-id" {
		t.Errorf("Access-Control-Expose-Headers = %q, want mcp-session-id", got)
	}
}

func TestCORSMiddleware_SpecificOrigin_NonMatchingRequest(t *testing.T) {
	handler := corsMiddleware("https://claude.ai", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "https://attacker.example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Request still passes through to the inner handler — browser enforces CORS
	// on its side when ACAO is absent; server-side blocking is not required.
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for non-matching origin", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Errorf("Access-Control-Allow-Methods = %q, want empty for non-matching origin", got)
	}
}

func TestCORSMiddleware_SpecificOrigin_NoOriginHeader(t *testing.T) {
	// Requests without an Origin header (curl, server-to-server) must not be
	// blocked and receive no CORS headers (they don't need them).
	var innerCalled bool
	handler := corsMiddleware("https://claude.ai", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil) // no Origin header
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !innerCalled {
		t.Error("inner handler not called for non-browser (no Origin) request")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty when Origin header absent", got)
	}
}

func TestCORSMiddleware_SpecificOrigin_PreflightMatching(t *testing.T) {
	const allowed = "https://claude.ai"
	handler := corsMiddleware(allowed, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("inner handler should not be called for OPTIONS preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", allowed)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowed)
	}
	if got := rr.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary = %q, want it to contain Origin", got)
	}
}

func TestCORSMiddleware_SpecificOrigin_PreflightNonMatching(t *testing.T) {
	handler := corsMiddleware("https://claude.ai", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("inner handler should not be called for OPTIONS preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "https://attacker.example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty — browser must block this preflight", got)
	}
}

func TestWithCORSOrigin(t *testing.T) {
	hs := &HTTPServer{}
	WithCORSOrigin("https://claude.ai")(hs)
	if hs.corsOrigin != "https://claude.ai" {
		t.Errorf("corsOrigin = %q, want https://claude.ai", hs.corsOrigin)
	}
}

// --- Bearer middleware tests ---

// testJWKSServer starts an httptest server serving a JWKS for key with kid.
func testJWKSServer(t *testing.T, kid string, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nB64 := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
		eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{"kty": "RSA", "kid": kid, "n": nB64, "e": eB64},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// testToken signs a JWT with key for the given claims.
func testToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func TestBearerMiddleware_MissingToken_Returns401(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwksSrv := testJWKSServer(t, "k1", key)
	v := auth.New("https://issuer.example.com", "make-mcp", jwksSrv.URL, &http.Client{})

	hs := &HTTPServer{validator: v}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := bearerMiddleware(hs, inner)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestBearerMiddleware_InvalidToken_Returns401(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwksSrv := testJWKSServer(t, "k1", key)
	v := auth.New("https://issuer.example.com", "make-mcp", jwksSrv.URL, &http.Client{})

	hs := &HTTPServer{validator: v}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := bearerMiddleware(hs, inner)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestBearerMiddleware_ValidToken_PassesThrough(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "k1"
	jwksSrv := testJWKSServer(t, kid, key)
	v := auth.New("https://issuer.example.com", "make-mcp", jwksSrv.URL, &http.Client{})

	hs := &HTTPServer{validator: v}

	var calledInner bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledInner = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := bearerMiddleware(hs, inner)

	token := testToken(t, key, kid, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": jwt.ClaimStrings{"make-mcp"},
		"exp": time.Now().Add(time.Hour).Unix(),
		"sub": "user-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if !calledInner {
		t.Error("inner handler was not called with valid token")
	}
}

func TestBearerMiddleware_NoValidator_PassesThrough(t *testing.T) {
	hs := &HTTPServer{validator: nil}
	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := bearerMiddleware(hs, inner)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("inner handler not called when OAuth disabled")
	}
}

func TestOAuthMetadata_ReturnsJSON(t *testing.T) {
	hs := NewHTTPServer(
		newToolServer(t, []parser.Recipe{validRecipe()}),
		"127.0.0.1:0",
		WithOAuthIssuer("https://auth.example.com/realms/make-mcp"),
	)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	req.Host = "localhost:9378"
	rr := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var doc map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&doc); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := doc["resource"]; !ok {
		t.Error("response missing 'resource' field")
	}
	if _, ok := doc["authorization_servers"]; !ok {
		t.Error("response missing 'authorization_servers' field")
	}
	if _, ok := doc["bearer_methods_supported"]; !ok {
		t.Error("response missing 'bearer_methods_supported' field")
	}
}

func TestOAuthMetadata_NotRegisteredWithoutIssuer(t *testing.T) {
	hs := NewHTTPServer(newToolServer(t, []parser.Recipe{validRecipe()}), "127.0.0.1:0")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rr := httptest.NewRecorder()
	hs.server.Handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Error("oauth metadata should not be registered without issuer or validator")
	}
}

func TestWithTLS_SetsCertAndKey(t *testing.T) {
	hs := &HTTPServer{}
	WithTLS(config.TLSConfig{Cert: "server.crt", Key: "server.key"})(hs)
	if hs.tlsCert != "server.crt" {
		t.Errorf("tlsCert = %q, want server.crt", hs.tlsCert)
	}
	if hs.tlsKey != "server.key" {
		t.Errorf("tlsKey = %q, want server.key", hs.tlsKey)
	}
}

func TestPropagateTraceContext_ExtractsTraceparent(t *testing.T) {
	// A real tracer provider is required: the no-op provider returns an invalid
	// span context, which would make the child span appear invalid even though
	// propagation succeeded.
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	var got trace.SpanContext
	handler := propagateTraceContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if !got.IsValid() {
		t.Fatal("expected span context to be valid")
	}
	// propagateTraceContext creates a child span: the trace ID must be inherited
	// from the incoming traceparent, but the span ID is the newly created child's.
	if got.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace id = %s, want 4bf92f3577b34da6a3ce929d0e0e4736", got.TraceID())
	}
	if got.SpanID().String() == "00f067aa0ba902b7" {
		t.Error("span id should be a new child span, not the incoming parent span id")
	}
}
