package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/parser"
	"go.opentelemetry.io/otel"
)

func TestPrometheusAddr_Defaults(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_HOST", "")
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_PORT", "")
	if got := prometheusAddr(); got != "127.0.0.1:9090" {
		t.Errorf("prometheusAddr() = %q, want 127.0.0.1:9090", got)
	}
}

func TestPrometheusAddr_EnvOverride(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_HOST", "0.0.0.0")
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_PORT", "8080")
	if got := prometheusAddr(); got != "0.0.0.0:8080" {
		t.Errorf("prometheusAddr() = %q, want 0.0.0.0:8080", got)
	}
}

func TestRecordToolsListRequest(t *testing.T) {
	RecordToolsListRequest(context.Background(), StatusSuccess, 5*time.Millisecond)
	RecordToolsListRequest(context.Background(), StatusFailure, 0)
}

func TestRecordToolsListBytes(t *testing.T) {
	// Verify no panic with the default no-op meter provider.
	RecordToolsListBytes(context.Background(), StatusSuccess, 0, 512)
	RecordToolsListBytes(context.Background(), StatusFailure, 0, 0)
}

func TestRecordToolInvocationBytes(t *testing.T) {
	recipe := parser.Recipe{ID: "greet", Risk: parser.RiskLow}
	RecordToolInvocationBytes(context.Background(), recipe, StatusSuccess, 20, 100)
	RecordToolInvocationBytes(context.Background(), recipe, StatusFailure, 20, 0)
}

func TestTracer(t *testing.T) {
	if Tracer() == nil {
		t.Error("Tracer() returned nil")
	}
}

func TestMetrics(t *testing.T) {
	m := Metrics()
	if m.ToolReloadTotal == nil {
		t.Error("ToolReloadTotal is nil")
	}
	if m.ToolInvocationsTotal == nil {
		t.Error("ToolInvocationsTotal is nil")
	}
	if m.ConnectedClients == nil {
		t.Error("ConnectedClients is nil")
	}
	if m.ToolsListRequestLatencySeconds == nil {
		t.Error("ToolsListRequestLatencySeconds is nil")
	}
	if m.ToolsListBytesIn == nil {
		t.Error("ToolsListBytesIn is nil")
	}
	if m.ToolsListBytesOut == nil {
		t.Error("ToolsListBytesOut is nil")
	}
	if m.ToolInvocationBytesIn == nil {
		t.Error("ToolInvocationBytesIn is nil")
	}
	if m.ToolInvocationBytesOut == nil {
		t.Error("ToolInvocationBytesOut is nil")
	}
}

func TestInitTraceProvider_Empty(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "")
	var fns []func(context.Context) error
	if err := initTraceProvider(context.Background(), nil, &fns); err != nil {
		t.Errorf("initTraceProvider('') error = %v", err)
	}
	if len(fns) != 0 {
		t.Errorf("expected no shutdown fns, got %d", len(fns))
	}
}

func TestInitTraceProvider_None(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	var fns []func(context.Context) error
	if err := initTraceProvider(context.Background(), nil, &fns); err != nil {
		t.Errorf("initTraceProvider(none) error = %v", err)
	}
}

func TestInitTraceProvider_OTLP(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	previousTracer := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previousTracer) })
	var fns []func(context.Context) error
	if err := initTraceProvider(context.Background(), nil, &fns); err != nil {
		t.Skipf("initTraceProvider(otlp): %v (no OTLP endpoint available)", err)
	}
	if len(fns) != 1 {
		t.Errorf("expected 1 shutdown fn, got %d", len(fns))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = fns[0](ctx)
}

func TestInitTraceProvider_Unsupported(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "zipkin")
	var fns []func(context.Context) error
	if err := initTraceProvider(context.Background(), nil, &fns); err == nil {
		t.Error("expected error for unsupported exporter, got nil")
	}
}

func TestInitMetricProvider_None(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	previous := otel.GetMeterProvider()
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = initMetrics()
	})
	var fns []func(context.Context) error
	if err := initMetricProvider(context.Background(), nil, &fns); err != nil {
		t.Errorf("initMetricProvider(none) error = %v", err)
	}
	if len(fns) != 0 {
		t.Errorf("expected no shutdown fns for none, got %d", len(fns))
	}
}

func TestInitMetricProvider_Prometheus(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "prometheus")
	t.Setenv("OTEL_EXPORTER_PROMETHEUS_PORT", "19091") // avoid conflict with default 9090
	previous := otel.GetMeterProvider()
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = initMetrics()
	})
	var fns []func(context.Context) error
	if err := initMetricProvider(context.Background(), nil, &fns); err != nil {
		t.Errorf("initMetricProvider(prometheus) error = %v", err)
	}
	if len(fns) != 2 {
		t.Errorf("expected 2 shutdown fns (provider + http server), got %d", len(fns))
	}
	// Shut down cleanly.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	for i := len(fns) - 1; i >= 0; i-- {
		_ = fns[i](shutdownCtx)
	}
}

func TestInitMetricProvider_OTLP(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	previous := otel.GetMeterProvider()
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = initMetrics()
	})
	var fns []func(context.Context) error
	if err := initMetricProvider(context.Background(), nil, &fns); err != nil {
		t.Skipf("initMetricProvider(otlp): %v", err)
	}
	if len(fns) != 1 {
		t.Errorf("expected 1 shutdown fn, got %d", len(fns))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = fns[0](ctx)
}

func TestInitMetricProvider_Unsupported(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "datadog")
	var fns []func(context.Context) error
	if err := initMetricProvider(context.Background(), nil, &fns); err == nil {
		t.Error("expected error for unsupported metric exporter, got nil")
	}
}

func TestInit_TraceProviderError(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "zipkin") // unsupported → initTraceProvider fails
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	if _, err := Init(context.Background()); err == nil {
		t.Error("expected error when OTEL_TRACES_EXPORTER=zipkin, got nil")
	}
}

func TestInit_MetricProviderError(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "datadog") // unsupported → initMetricProvider fails
	if _, err := Init(context.Background()); err == nil {
		t.Error("expected error when OTEL_METRICS_EXPORTER=datadog, got nil")
	}
}

func TestInit_NoneExporters(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	previous := otel.GetMeterProvider()
	previousTracer := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		otel.SetTracerProvider(previousTracer)
		_ = initMetrics()
	})

	shutdown, err := Init(context.Background())
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown fn")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown() error = %v", err)
	}
}
