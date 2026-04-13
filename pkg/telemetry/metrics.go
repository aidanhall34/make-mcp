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
	ToolsRegistered                metric.Int64Gauge

	// Auth instruments
	AuthAttemptsTotal   metric.Int64Counter
	AuthAttemptDuration metric.Float64Histogram

	// Resource list instruments
	ResourcesListedTotal         metric.Int64Counter
	ResourcesListRequestDuration metric.Float64Histogram
	ResourcesListFileCount       metric.Int64Histogram

	// Resource read instruments
	ResourcesReadTotal     metric.Int64Counter
	ResourcesReadDuration  metric.Float64Histogram
	ResourcesReadFileCount metric.Int64Histogram
	ResourcesReadBytesIn   metric.Int64Histogram
	ResourcesReadBytesOut  metric.Int64Histogram

	// File watch instruments
	FilesWatched      metric.Int64Gauge
	FilesWatchedTotal metric.Int64Counter

	// Subscription instruments
	ResourceSubscriptionsTotal  metric.Int64Counter
	ResourceSubscriptionsActive metric.Int64UpDownCounter

	// Notification instruments
	ResourceNotificationDuration metric.Float64Histogram
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
	if m.ToolsRegistered, err = meter.Int64Gauge(
		"make_mcp_tools_registered",
		metric.WithDescription("Current number of registered MCP tools."),
	); err != nil {
		return err
	}

	// Auth instruments
	if m.AuthAttemptsTotal, err = meter.Int64Counter(
		"make_mcp_auth_attempts_total",
		metric.WithDescription("Total number of OAuth token validation attempts."),
	); err != nil {
		return err
	}
	if m.AuthAttemptDuration, err = meter.Float64Histogram(
		"make_mcp_auth_attempt_duration_seconds",
		metric.WithDescription("Latency of OAuth token validation attempts."),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}

	// Resource list instruments
	if m.ResourcesListedTotal, err = meter.Int64Counter(
		"make_mcp_resources_listed_total",
		metric.WithDescription("Total number of resources returned across all resources/list requests."),
	); err != nil {
		return err
	}
	if m.ResourcesListRequestDuration, err = meter.Float64Histogram(
		"make_mcp_resources_list_request_duration_seconds",
		metric.WithDescription("Server-side latency of resources/list requests."),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}
	if m.ResourcesListFileCount, err = meter.Int64Histogram(
		"make_mcp_resources_list_file_count",
		metric.WithDescription("Number of resources returned per resources/list request."),
	); err != nil {
		return err
	}

	// Resource read instruments
	if m.ResourcesReadTotal, err = meter.Int64Counter(
		"make_mcp_resources_read_total",
		metric.WithDescription("Total number of resources/read requests."),
	); err != nil {
		return err
	}
	if m.ResourcesReadDuration, err = meter.Float64Histogram(
		"make_mcp_resources_read_duration_seconds",
		metric.WithDescription("Server-side latency of resources/read requests."),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}
	if m.ResourcesReadFileCount, err = meter.Int64Histogram(
		"make_mcp_resources_read_file_count",
		metric.WithDescription("Number of file contents returned per resources/read response."),
	); err != nil {
		return err
	}
	if m.ResourcesReadBytesIn, err = meter.Int64Histogram(
		"make_mcp_resources_read_bytes_in",
		metric.WithDescription("Bytes received in resources/read requests (URI length)."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if m.ResourcesReadBytesOut, err = meter.Int64Histogram(
		"make_mcp_resources_read_bytes_out",
		metric.WithDescription("Bytes sent in resources/read responses (file content size)."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}

	// File watch instruments
	if m.FilesWatched, err = meter.Int64Gauge(
		"make_mcp_files_watched",
		metric.WithDescription("Current number of files registered as MCP resources."),
	); err != nil {
		return err
	}
	if m.FilesWatchedTotal, err = meter.Int64Counter(
		"make_mcp_files_watched_total",
		metric.WithDescription("Total number of files ever registered as MCP resources (including removed)."),
	); err != nil {
		return err
	}

	// Subscription instruments
	if m.ResourceSubscriptionsTotal, err = meter.Int64Counter(
		"make_mcp_resource_subscriptions_total",
		metric.WithDescription("Total number of resource subscription events (subscribe/unsubscribe)."),
	); err != nil {
		return err
	}
	if m.ResourceSubscriptionsActive, err = meter.Int64UpDownCounter(
		"make_mcp_resource_subscriptions_active",
		metric.WithDescription("Current number of active resource subscriptions."),
	); err != nil {
		return err
	}

	// Notification instruments
	if m.ResourceNotificationDuration, err = meter.Float64Histogram(
		"make_mcp_resource_notification_duration_seconds",
		metric.WithDescription("Latency of sending resource update notifications to subscribed clients."),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}

	metricsMu.Lock()
	metrics = m
	metricsMu.Unlock()
	return nil
}

// RecordAuthAttempt records the outcome and latency of a Bearer token validation.
func RecordAuthAttempt(ctx context.Context, status string, d time.Duration) {
	m := Metrics()
	attrs := metric.WithAttributes(attribute.String("status", status))
	m.AuthAttemptsTotal.Add(ctx, 1, attrs)
	m.AuthAttemptDuration.Record(ctx, d.Seconds(), attrs)
}

// RecordResourcesListRequest records a resources/list request with its latency
// and the number of resources returned.
func RecordResourcesListRequest(ctx context.Context, status string, d time.Duration, fileCount int64) {
	m := Metrics()
	attrs := metric.WithAttributes(attribute.String("status", status))
	m.ResourcesListedTotal.Add(ctx, fileCount, attrs)
	m.ResourcesListRequestDuration.Record(ctx, d.Seconds(), attrs)
	m.ResourcesListFileCount.Record(ctx, fileCount, attrs)
}

// RecordResourcesRead records a resources/read request with its latency,
// number of content items returned, and bytes transferred.
func RecordResourcesRead(ctx context.Context, status string, d time.Duration, fileCount, bytesIn, bytesOut int64) {
	m := Metrics()
	attrs := metric.WithAttributes(attribute.String("status", status))
	m.ResourcesReadTotal.Add(ctx, 1, attrs)
	m.ResourcesReadDuration.Record(ctx, d.Seconds(), attrs)
	m.ResourcesReadFileCount.Record(ctx, fileCount, attrs)
	m.ResourcesReadBytesIn.Record(ctx, bytesIn, attrs)
	m.ResourcesReadBytesOut.Record(ctx, bytesOut, attrs)
}

// RecordFilesWatched sets the current watched-file gauge and increments the
// total counter. delta should be +1 when a file is added, -1 when removed.
func RecordFilesWatched(ctx context.Context, delta int64) {
	m := Metrics()
	m.FilesWatched.Record(ctx, delta)
	if delta > 0 {
		m.FilesWatchedTotal.Add(ctx, delta)
	}
}

// RecordResourceSubscription records a subscribe or unsubscribe event.
// delta should be +1 for subscribe, -1 for unsubscribe.
func RecordResourceSubscription(ctx context.Context, action string, delta int64) {
	m := Metrics()
	attrs := metric.WithAttributes(attribute.String("action", action))
	m.ResourceSubscriptionsTotal.Add(ctx, 1, attrs)
	m.ResourceSubscriptionsActive.Add(ctx, delta)
}

// RecordResourceNotification records the latency of sending a resource update
// notification.
func RecordResourceNotification(ctx context.Context, status string, d time.Duration) {
	m := Metrics()
	m.ResourceNotificationDuration.Record(ctx, d.Seconds(),
		metric.WithAttributes(attribute.String("status", status)),
	)
}

func RecordToolReload(ctx context.Context, file, status string) {
	Metrics().ToolReloadTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("file", file),
		attribute.String("status", status),
	))
}

func RecordToolInvocation(ctx context.Context, recipe parser.Recipe, status string, elapsed time.Duration, streaming bool) {
	m := Metrics()
	attrs := metric.WithAttributes(recipeAttrs(recipe, status, streaming)...)
	m.ToolInvocationsTotal.Add(ctx, 1, attrs)
	m.ToolInvocationDuration.Record(ctx, elapsed.Seconds(), attrs)
}

// RecordToolsListRequest records the server-side latency and count of a
// tools/list request. status must be StatusSuccess or StatusFailure.
func RecordToolsListRequest(ctx context.Context, status string, d time.Duration) {
	m := Metrics()
	attrs := metric.WithAttributes(attribute.String("status", status))
	m.ToolsListRequestsTotal.Add(ctx, 1, attrs)
	m.ToolsListRequestLatencySeconds.Record(ctx, d.Seconds(), attrs)
}

// RecordToolsListed increments the total tools-listed counter by one for each
// recipe, labeled by risk level and MCP tool hints.
func RecordToolsListed(ctx context.Context, recipes []parser.Recipe) {
	m := Metrics()
	for _, r := range recipes {
		m.ToolsListedTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("risk", string(r.Risk)),
			attribute.Bool("read_only", resolveHint(r.ToolHints.ReadOnly, false)),
			attribute.Bool("destructive", resolveHint(r.ToolHints.Destructive, r.Risk == parser.RiskHigh)),
			attribute.Bool("idempotent", resolveHint(r.ToolHints.Idempotent, false)),
			attribute.Bool("open_world", resolveHint(r.ToolHints.OpenWorld, true)),
		))
	}
}

// RecordToolsRegistered sets the current registered-tool-count gauge, emitting
// one value per unique {risk, hints} combination so dashboards can break down
// the registered tool set by type.
func RecordToolsRegistered(ctx context.Context, recipes []parser.Recipe) {
	type groupKey struct {
		risk        string
		readOnly    bool
		destructive bool
		idempotent  bool
		openWorld   bool
	}
	counts := map[groupKey]int64{}
	for _, r := range recipes {
		counts[groupKey{
			risk:        string(r.Risk),
			readOnly:    resolveHint(r.ToolHints.ReadOnly, false),
			destructive: resolveHint(r.ToolHints.Destructive, r.Risk == parser.RiskHigh),
			idempotent:  resolveHint(r.ToolHints.Idempotent, false),
			openWorld:   resolveHint(r.ToolHints.OpenWorld, true),
		}]++
	}
	m := Metrics()
	for k, count := range counts {
		m.ToolsRegistered.Record(ctx, count, metric.WithAttributes(
			attribute.String("risk", k.risk),
			attribute.Bool("read_only", k.readOnly),
			attribute.Bool("destructive", k.destructive),
			attribute.Bool("idempotent", k.idempotent),
			attribute.Bool("open_world", k.openWorld),
		))
	}
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
func RecordToolInvocationBytes(ctx context.Context, recipe parser.Recipe, status string, bytesIn, bytesOut int64, streaming bool) {
	m := Metrics()
	attrs := metric.WithAttributes(recipeAttrs(recipe, status, streaming)...)
	m.ToolInvocationBytesIn.Record(ctx, bytesIn, attrs)
	m.ToolInvocationBytesOut.Record(ctx, bytesOut, attrs)
}

// recipeAttrs returns the standard attribute set for a tool invocation,
// including risk and all resolved MCP tool hints as boolean labels.
func recipeAttrs(recipe parser.Recipe, status string, streaming bool) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("tool", recipe.ID),
		attribute.String("risk", string(recipe.Risk)),
		attribute.Bool("read_only", resolveHint(recipe.ToolHints.ReadOnly, false)),
		attribute.Bool("destructive", resolveHint(recipe.ToolHints.Destructive, recipe.Risk == parser.RiskHigh)),
		attribute.Bool("idempotent", resolveHint(recipe.ToolHints.Idempotent, false)),
		attribute.Bool("open_world", resolveHint(recipe.ToolHints.OpenWorld, true)),
		attribute.String("status", status),
		attribute.Bool("streaming", streaming),
	}
}

// resolveHint returns the value of a *bool hint, falling back to defaultValue
// when the hint is nil (not explicitly annotated).
func resolveHint(hint *bool, defaultValue bool) bool {
	if hint != nil {
		return *hint
	}
	return defaultValue
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
