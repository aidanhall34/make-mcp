package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
	rootserver "github.com/aidanhall34/make-mcp/pkg/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
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

func TestPropagateTraceContext_ExtractsTraceparent(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTextMapPropagator(previous)
	})

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
		t.Fatal("expected extracted span context to be valid")
	}
	if got.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace id = %s, want %s", got.TraceID(), "4bf92f3577b34da6a3ce929d0e0e4736")
	}
	if got.SpanID().String() != "00f067aa0ba902b7" {
		t.Errorf("span id = %s, want %s", got.SpanID(), "00f067aa0ba902b7")
	}
}
