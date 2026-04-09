package telemetry

import (
	"context"
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
	ToolsListedTotal         metric.Int64Counter
	ToolsListRequestsTotal   metric.Int64Counter
	ToolsListDurationSeconds metric.Float64Histogram
	ToolReloadTotal          metric.Int64Counter
	ToolInvocationsTotal     metric.Int64Counter
	ToolInvocationDuration   metric.Float64Histogram
	ConnectedClients         metric.Int64UpDownCounter
}

var metrics Instruments

func init() {
	_ = initMetrics()
}

// initMetrics creates all metric instruments on the global provider.
func initMetrics() error {
	meter := otel.Meter(instrumentationName)
	var err error

	if metrics.ToolsListedTotal, err = meter.Int64Counter("make_mcp_tools_listed_total"); err != nil {
		return err
	}
	if metrics.ToolsListRequestsTotal, err = meter.Int64Counter("make_mcp_tools_list_requests_total"); err != nil {
		return err
	}
	if metrics.ToolsListDurationSeconds, err = meter.Float64Histogram("make_mcp_tools_list_duration_seconds"); err != nil {
		return err
	}
	if metrics.ToolReloadTotal, err = meter.Int64Counter("make_mcp_tool_reload_total"); err != nil {
		return err
	}
	if metrics.ToolInvocationsTotal, err = meter.Int64Counter("make_mcp_tool_invocations_total"); err != nil {
		return err
	}
	if metrics.ToolInvocationDuration, err = meter.Float64Histogram("make_mcp_tool_invocation_duration_seconds"); err != nil {
		return err
	}
	if metrics.ConnectedClients, err = meter.Int64UpDownCounter("make_mcp_connected_clients"); err != nil {
		return err
	}

	return nil
}

func RecordToolReload(ctx context.Context, file, status string) {
	metrics.ToolReloadTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("file", file),
		attribute.String("status", status),
	))
}

func RecordToolInvocation(ctx context.Context, recipe parser.Recipe, status string, elapsed time.Duration) {
	attrs := metric.WithAttributes(
		attribute.String("tool", recipe.ID),
		attribute.String("risk", string(recipe.Risk)),
		attribute.String("status", status),
	)
	metrics.ToolInvocationsTotal.Add(ctx, 1, attrs)
	metrics.ToolInvocationDuration.Record(ctx, elapsed.Seconds(), attrs)
}

// Metrics returns the initialized metric instruments.
func Metrics() Instruments {
	return metrics
}

// Tracer returns the shared tracer.
func Tracer() trace.Tracer {
	return otel.Tracer(instrumentationName)
}
