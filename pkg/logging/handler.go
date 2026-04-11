// Package logging provides a slog.Handler wrapper that enriches every log
// record with the active OpenTelemetry trace_id and span_id extracted from
// the context. Records produced outside of a valid span are passed through
// unchanged.
package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler wraps an inner slog.Handler and appends trace_id and span_id
// string attributes to every record whose context carries a valid OTel span.
// All other Handler methods delegate to the inner handler unchanged.
type TraceHandler struct {
	inner slog.Handler
}

// NewTraceHandler returns a TraceHandler that wraps h.
func NewTraceHandler(h slog.Handler) *TraceHandler {
	return &TraceHandler{inner: h}
}

// Enabled reports whether the inner handler would emit a record at the given level.
func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle adds trace_id and span_id to r when ctx contains a valid OTel span,
// then delegates to the inner handler.
func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new TraceHandler whose inner handler has the given attrs.
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup returns a new TraceHandler whose inner handler has the given group.
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{inner: h.inner.WithGroup(name)}
}
