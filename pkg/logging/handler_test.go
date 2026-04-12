package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/aidanhall34/make-mcp/pkg/logging"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// captureHandler records the most recently handled record's extra attributes.
type captureHandler struct {
	buf *bytes.Buffer
}

func newCaptureHandler() *captureHandler {
	return &captureHandler{buf: &bytes.Buffer{}}
}

func (h *captureHandler) handler() slog.Handler {
	return slog.NewJSONHandler(h.buf, &slog.HandlerOptions{Level: slog.LevelDebug})
}

// logLine unmarshals the latest JSON line from the buffer.
func (h *captureHandler) logLine(t *testing.T) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(h.buf.String()))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode log line: %v (raw: %q)", err, h.buf.String())
	}
	return m
}

func TestTraceHandler_AddsTraceAndSpanID(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(logging.NewTraceHandler(cap.handler()))

	// Create a real in-memory tracer so we get non-zero IDs.
	exp := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exp))
	ctx, span := tp.Tracer("test").Start(context.Background(), "root")
	defer span.End()

	logger.InfoContext(ctx, "hello")

	m := cap.logLine(t)
	traceID, hasTrace := m["trace_id"].(string)
	spanID, hasSpan := m["span_id"].(string)

	if !hasTrace || traceID == "" || traceID == "00000000000000000000000000000000" {
		t.Errorf("expected non-zero trace_id, got %q", traceID)
	}
	if !hasSpan || spanID == "" || spanID == "0000000000000000" {
		t.Errorf("expected non-zero span_id, got %q", spanID)
	}
	// IDs must match the active span.
	sc := span.SpanContext()
	if traceID != sc.TraceID().String() {
		t.Errorf("trace_id = %q, want %q", traceID, sc.TraceID().String())
	}
	if spanID != sc.SpanID().String() {
		t.Errorf("span_id = %q, want %q", spanID, sc.SpanID().String())
	}
}

func TestTraceHandler_NoSpanPassesThrough(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(logging.NewTraceHandler(cap.handler()))

	// Background context has no active span — record must still be emitted.
	logger.InfoContext(context.Background(), "no span")

	m := cap.logLine(t)
	if _, has := m["trace_id"]; has {
		t.Error("expected no trace_id when context has no span")
	}
	if _, has := m["span_id"]; has {
		t.Error("expected no span_id when context has no span")
	}
	if m["msg"] != "no span" {
		t.Errorf("msg = %q, want %q", m["msg"], "no span")
	}
}

func TestTraceHandler_WithAttrsPreservesTracing(t *testing.T) {
	cap := newCaptureHandler()
	base := slog.New(logging.NewTraceHandler(cap.handler()))
	logger := base.With("service", "test")

	exp := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exp))
	ctx, span := tp.Tracer("test").Start(context.Background(), "root")
	defer span.End()

	logger.InfoContext(ctx, "with attrs")

	m := cap.logLine(t)
	if _, has := m["trace_id"]; !has {
		t.Error("trace_id missing after WithAttrs")
	}
	if m["service"] != "test" {
		t.Errorf("service attr = %q, want %q", m["service"], "test")
	}
}

func TestTraceHandler_WithGroupPreservesTracing(t *testing.T) {
	cap := newCaptureHandler()
	base := slog.New(logging.NewTraceHandler(cap.handler()))
	logger := base.WithGroup("grp")

	exp := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exp))
	ctx, span := tp.Tracer("test").Start(context.Background(), "root")
	defer span.End()

	logger.InfoContext(ctx, "with group")

	// When WithGroup is active, slog.JSONHandler nests record attrs under the
	// group key — including trace_id/span_id added by Handle. Check raw output.
	raw := cap.buf.String()
	if !strings.Contains(raw, "trace_id") {
		t.Error("trace_id absent from log output after WithGroup")
	}
	if !strings.Contains(raw, "span_id") {
		t.Error("span_id absent from log output after WithGroup")
	}
}

func TestTraceHandler_EnabledDelegates(t *testing.T) {
	inner := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := logging.NewTraceHandler(inner)

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Enabled(Debug) = true, want false when inner level is Warn")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled(Error) = false, want true when inner level is Warn")
	}
}
