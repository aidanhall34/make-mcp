package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/parser"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/aidanhall34/make-mcp"

const (
	StatusSuccess = "success"
	StatusFailure = "fail"
)

// Instruments groups the metric instruments used across the server.
type Instruments struct {
	ToolsListedTotal               metric.Int64Counter
	ToolsListRequestsTotal         metric.Int64Counter
	ToolsListRequestLatencySeconds metric.Float64Histogram
	ToolsListBytesIn               metric.Int64Histogram
	ToolsListBytesOut              metric.Int64Histogram
	ToolReloadTotal                metric.Int64Counter
	ToolInvocationsTotal           metric.Int64Counter
	ToolInvocationDuration         metric.Float64Histogram
	ToolInvocationBytesIn          metric.Int64Histogram
	ToolInvocationBytesOut         metric.Int64Histogram
	ConnectedClients               metric.Int64UpDownCounter
}

var (
	metricsMu sync.RWMutex
	metrics   Instruments
)

func init() {
	_ = initMetrics()
}

// initMetrics creates all metric instruments on the global provider.
func initMetrics() error {
	meter := otel.Meter(instrumentationName)
	var (
		err error
		m   Instruments
	)

	if m.ToolsListedTotal, err = meter.Int64Counter("make_mcp_tools_listed_total"); err != nil {
		return err
	}
	if m.ToolsListRequestsTotal, err = meter.Int64Counter("make_mcp_tools_list_requests_total"); err != nil {
		return err
	}
	if m.ToolsListRequestLatencySeconds, err = meter.Float64Histogram(
		"make_mcp_tools_list_request_latency_seconds",
		metric.WithDescription("Server-side latency of tools/list requests."),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}
	if m.ToolsListBytesIn, err = meter.Int64Histogram(
		"make_mcp_tools_list_request_bytes_in",
		metric.WithDescription("Bytes received in tools/list requests (cursor size)."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if m.ToolsListBytesOut, err = meter.Int64Histogram(
		"make_mcp_tools_list_request_bytes_out",
		metric.WithDescription("Bytes sent in tools/list responses (JSON-encoded tool definitions)."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if m.ToolReloadTotal, err = meter.Int64Counter("make_mcp_tool_reload_total"); err != nil {
		return err
	}
	if m.ToolInvocationsTotal, err = meter.Int64Counter("make_mcp_tool_invocations_total"); err != nil {
		return err
	}
	if m.ToolInvocationDuration, err = meter.Float64Histogram("make_mcp_tool_invocation_duration_seconds"); err != nil {
		return err
	}
	if m.ToolInvocationBytesIn, err = meter.Int64Histogram(
		"make_mcp_tool_invocation_bytes_in",
		metric.WithDescription("Bytes received in tool invocation requests (JSON-encoded arguments)."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if m.ToolInvocationBytesOut, err = meter.Int64Histogram(
		"make_mcp_tool_invocation_bytes_out",
		metric.WithDescription("Bytes sent in tool invocation responses (stdout + stderr)."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if m.ConnectedClients, err = meter.Int64UpDownCounter("make_mcp_connected_clients"); err != nil {
		return err
	}

	metricsMu.Lock()
	metrics = m
	metricsMu.Unlock()
	return nil
}

func RecordToolReload(ctx context.Context, file, status string) {
	Metrics().ToolReloadTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("file", file),
		attribute.String("status", status),
	))
}

func RecordToolInvocation(ctx context.Context, recipe parser.Recipe, status string, elapsed time.Duration) {
	m := Metrics()
	attrs := metric.WithAttributes(
		attribute.String("tool", recipe.ID),
		attribute.String("risk", string(recipe.Risk)),
		attribute.String("status", status),
	)
	m.ToolInvocationsTotal.Add(ctx, 1, attrs)
	m.ToolInvocationDuration.Record(ctx, elapsed.Seconds(), attrs)
}

// RecordToolsListRequest records the server-side latency of a tools/list
// request. status must be StatusSuccess or StatusFailure.
func RecordToolsListRequest(ctx context.Context, status string, d time.Duration) {
	Metrics().ToolsListRequestLatencySeconds.Record(ctx, d.Seconds(),
		metric.WithAttributes(attribute.String("status", status)),
	)
}

// RecordToolsListBytes records the bytes received and sent for a tools/list
// request. status must be StatusSuccess or StatusFailure.
func RecordToolsListBytes(ctx context.Context, status string, bytesIn, bytesOut int64) {
	m := Metrics()
	attrs := metric.WithAttributes(attribute.String("status", status))
	m.ToolsListBytesIn.Record(ctx, bytesIn, attrs)
	m.ToolsListBytesOut.Record(ctx, bytesOut, attrs)
}

// RecordToolInvocationBytes records the bytes received and sent for a tool
// invocation. status must be StatusSuccess or StatusFailure.
func RecordToolInvocationBytes(ctx context.Context, recipe parser.Recipe, status string, bytesIn, bytesOut int64) {
	m := Metrics()
	attrs := metric.WithAttributes(
		attribute.String("tool", recipe.ID),
		attribute.String("status", status),
	)
	m.ToolInvocationBytesIn.Record(ctx, bytesIn, attrs)
	m.ToolInvocationBytesOut.Record(ctx, bytesOut, attrs)
}

// Metrics returns the initialized metric instruments.
func Metrics() Instruments {
	metricsMu.RLock()
	defer metricsMu.RUnlock()
	return metrics
}

// Tracer returns the shared tracer.
func Tracer() trace.Tracer {
	return otel.Tracer(instrumentationName)
}
