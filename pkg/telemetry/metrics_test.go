package telemetry

import (
	"context"
	"strconv"
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

	RecordToolInvocation(context.Background(), recipe, StatusFailure, 125*time.Millisecond, false)

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

	RecordToolInvocation(context.Background(), recipe, StatusSuccess, 125*time.Millisecond, false)

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

func TestRecordToolInvocationIncludesBooleanHintLabels(t *testing.T) {
	reader := setupManualMeter(t)
	ro := true
	recipe := parser.Recipe{
		ID:   "build",
		Risk: parser.RiskLow,
		ToolHints: parser.ToolHints{
			ReadOnly:   &ro,
			Idempotent: func() *bool { b := true; return &b }(),
		},
	}

	RecordToolInvocation(context.Background(), recipe, StatusSuccess, 10*time.Millisecond, false)

	rm := collectMetrics(t, reader)
	invocations := findSum[int64](t, rm, "make_mcp_tool_invocations_total")

	// Boolean hints must appear as labels; unlisted hints fall back to defaults.
	labels := map[string]string{
		"tool":        recipe.ID,
		"risk":        string(recipe.Risk),
		"status":      StatusSuccess,
		"read_only":   "true",
		"destructive": "false", // default for low-risk
		"idempotent":  "true",
		"open_world":  "true", // default
		"streaming":   "false",
	}
	if !hasInt64Point(invocations, labels, 1) {
		t.Fatalf("invocation counter missing point with boolean hint labels: %#v", invocations.DataPoints)
	}
}

func TestRecordToolsListedIncrementsCounter(t *testing.T) {
	reader := setupManualMeter(t)

	low := parser.RiskLow
	high := parser.RiskHigh
	recipes := []parser.Recipe{
		{ID: "build", Risk: low},
		{ID: "test", Risk: low},
		{ID: "deploy", Risk: low},
		{ID: "clean", Risk: low},
		{ID: "nuke", Risk: low},
		{ID: "a", Risk: high},
		{ID: "b", Risk: high},
		{ID: "c", Risk: high},
	}
	RecordToolsListed(context.Background(), recipes[:5])
	RecordToolsListed(context.Background(), recipes[5:])

	rm := collectMetrics(t, reader)
	counter := findSum[int64](t, rm, "make_mcp_tools_listed_total")

	if len(counter.DataPoints) == 0 {
		t.Fatalf("tools_listed_total has no data points")
	}
	total := int64(0)
	for _, dp := range counter.DataPoints {
		total += dp.Value
	}
	if total != 8 {
		t.Errorf("tools_listed_total = %d, want 8", total)
	}

	// Verify that risk labels are present.
	lowLabels := map[string]string{"risk": string(low), "read_only": "false", "destructive": "false", "idempotent": "false", "open_world": "true"}
	highLabels := map[string]string{"risk": string(high), "read_only": "false", "destructive": "true", "idempotent": "false", "open_world": "true"}
	if !hasInt64Point(counter, lowLabels, 5) {
		t.Errorf("tools_listed_total missing low-risk point: %#v", counter.DataPoints)
	}
	if !hasInt64Point(counter, highLabels, 3) {
		t.Errorf("tools_listed_total missing high-risk point: %#v", counter.DataPoints)
	}
}

func TestRecordToolsRegisteredSetsGauge(t *testing.T) {
	reader := setupManualMeter(t)

	low := parser.RiskLow
	high := parser.RiskHigh
	// 4 low-risk + 3 high-risk recipes; all have default hints.
	recipes := []parser.Recipe{
		{ID: "a", Risk: low},
		{ID: "b", Risk: low},
		{ID: "c", Risk: low},
		{ID: "d", Risk: low},
		{ID: "e", Risk: high},
		{ID: "f", Risk: high},
		{ID: "g", Risk: high},
	}
	RecordToolsRegistered(context.Background(), recipes)

	rm := collectMetrics(t, reader)
	gauge := findGauge[int64](t, rm, "make_mcp_tools_registered")

	if len(gauge.DataPoints) == 0 {
		t.Fatalf("tools_registered gauge has no data points")
	}

	total := int64(0)
	for _, dp := range gauge.DataPoints {
		total += dp.Value
	}
	if total != 7 {
		t.Errorf("tools_registered total = %d, want 7", total)
	}

	// Each risk group should have its own gauge point.
	lowLabels := map[string]string{"risk": string(low), "read_only": "false", "destructive": "false", "idempotent": "false", "open_world": "true"}
	highLabels := map[string]string{"risk": string(high), "read_only": "false", "destructive": "true", "idempotent": "false", "open_world": "true"}
	if !hasInt64GaugePoint(gauge, lowLabels, 4) {
		t.Errorf("tools_registered missing low-risk point with value 4: %#v", gauge.DataPoints)
	}
	if !hasInt64GaugePoint(gauge, highLabels, 3) {
		t.Errorf("tools_registered missing high-risk point with value 3: %#v", gauge.DataPoints)
	}
}

func TestRecordToolsListRequestIncrementsCounter(t *testing.T) {
	reader := setupManualMeter(t)

	RecordToolsListRequest(context.Background(), StatusSuccess, 5*time.Millisecond)
	RecordToolsListRequest(context.Background(), StatusSuccess, 3*time.Millisecond)

	rm := collectMetrics(t, reader)
	counter := findSum[int64](t, rm, "make_mcp_tools_list_requests_total")

	if !hasInt64Point(counter, map[string]string{"status": StatusSuccess}, 2) {
		t.Fatalf("tools_list_requests_total missing success point: %#v", counter.DataPoints)
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

func hasInt64GaugePoint(gauge metricdata.Gauge[int64], wantLabels map[string]string, wantValue int64) bool {
	for _, point := range gauge.DataPoints {
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

func findGauge[N int64 | float64](t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Gauge[N] {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				gauge, ok := m.Data.(metricdata.Gauge[N])
				if !ok {
					t.Fatalf("%s data type = %T, want metricdata.Gauge", name, m.Data)
				}
				return gauge
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Gauge[N]{}
}

func pointHasLabels(attrs attribute.Set, want map[string]string) bool {
	for key, value := range want {
		got, ok := attrs.Value(attribute.Key(key))
		if !ok {
			return false
		}
		var gotStr string
		switch got.Type() {
		case attribute.BOOL:
			gotStr = strconv.FormatBool(got.AsBool())
		case attribute.INT64:
			gotStr = strconv.FormatInt(got.AsInt64(), 10)
		case attribute.FLOAT64:
			gotStr = strconv.FormatFloat(got.AsFloat64(), 'f', -1, 64)
		default:
			gotStr = got.AsString()
		}
		if gotStr != value {
			return false
		}
	}
	return true
}
