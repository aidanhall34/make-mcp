package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/parser"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRecordToolReloadAddsStatusLabel(t *testing.T) {
	reader := setupManualMeter(t)

	RecordToolReload(context.Background(), "makefile", StatusSuccess)
	RecordToolReload(context.Background(), "makefile", StatusFailure)

	rm := collectMetrics(t, reader)
	counter := findSum[int64](t, rm, "make_mcp_tool_reload_total")

	if !hasInt64Point(counter, map[string]string{"file": "makefile", "status": StatusSuccess}, 1) {
		t.Fatalf("reload counter missing success point: %#v", counter.DataPoints)
	}
	if !hasInt64Point(counter, map[string]string{"file": "makefile", "status": StatusFailure}, 1) {
		t.Fatalf("reload counter missing failure point: %#v", counter.DataPoints)
	}
}

func TestRecordToolInvocationDurationAddsStatusLabel(t *testing.T) {
	reader := setupManualMeter(t)
	recipe := parser.Recipe{ID: "hello-world", Risk: parser.RiskLow}

	RecordToolInvocation(context.Background(), recipe, StatusFailure, 125*time.Millisecond)

	rm := collectMetrics(t, reader)
	invocations := findSum[int64](t, rm, "make_mcp_tool_invocations_total")
	durations := findHistogram[float64](t, rm, "make_mcp_tool_invocation_duration_seconds")

	labels := map[string]string{"tool": recipe.ID, "risk": string(recipe.Risk), "status": StatusFailure}
	if !hasInt64Point(invocations, labels, 1) {
		t.Fatalf("invocation counter missing failure point: %#v", invocations.DataPoints)
	}
	if !hasFloat64HistogramPoint(durations, labels, 1) {
		t.Fatalf("invocation duration histogram missing failure point: %#v", durations.DataPoints)
	}
}

func TestRecordedMetricsRemainPresentAcrossCumulativeCollections(t *testing.T) {
	reader := setupManualMeter(t)
	recipe := parser.Recipe{ID: "hello-world", Risk: parser.RiskLow}
	labels := map[string]string{"tool": recipe.ID, "risk": string(recipe.Risk), "status": StatusSuccess}

	RecordToolInvocation(context.Background(), recipe, StatusSuccess, 125*time.Millisecond)

	_ = collectMetrics(t, reader)
	second := collectMetrics(t, reader)
	invocations := findSum[int64](t, second, "make_mcp_tool_invocations_total")
	durations := findHistogram[float64](t, second, "make_mcp_tool_invocation_duration_seconds")

	if !hasInt64Point(invocations, labels, 1) {
		t.Fatalf("invocation counter was not present on second collection: %#v", invocations.DataPoints)
	}
	if !hasFloat64HistogramPoint(durations, labels, 1) {
		t.Fatalf("invocation duration histogram was not present on second collection: %#v", durations.DataPoints)
	}
}

func setupManualMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	if err := initMetrics(); err != nil {
		t.Fatalf("init metrics: %v", err)
	}
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetMeterProvider(previous)
		_ = initMetrics()
	})
	return reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return rm
}

func findSum[N int64 | float64](t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Sum[N] {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				sum, ok := m.Data.(metricdata.Sum[N])
				if !ok {
					t.Fatalf("%s data type = %T, want metricdata.Sum", name, m.Data)
				}
				return sum
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Sum[N]{}
}

func findHistogram[N int64 | float64](t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Histogram[N] {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				histogram, ok := m.Data.(metricdata.Histogram[N])
				if !ok {
					t.Fatalf("%s data type = %T, want metricdata.Histogram", name, m.Data)
				}
				return histogram
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Histogram[N]{}
}

func hasInt64Point(sum metricdata.Sum[int64], wantLabels map[string]string, wantValue int64) bool {
	for _, point := range sum.DataPoints {
		if point.Value == wantValue && pointHasLabels(point.Attributes, wantLabels) {
			return true
		}
	}
	return false
}

func hasFloat64HistogramPoint(histogram metricdata.Histogram[float64], wantLabels map[string]string, wantCount uint64) bool {
	for _, point := range histogram.DataPoints {
		if point.Count == wantCount && pointHasLabels(point.Attributes, wantLabels) {
			return true
		}
	}
	return false
}

func pointHasLabels(attrs attribute.Set, want map[string]string) bool {
	for key, value := range want {
		got, ok := attrs.Value(attribute.Key(key))
		if !ok || got.AsString() != value {
			return false
		}
	}
	return true
}
