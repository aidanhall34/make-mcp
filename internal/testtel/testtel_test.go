package testtel_test

import (
	"context"
	"testing"

	"github.com/aidanhall34/make-mcp/internal/testtel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setupTestProviders installs in-memory trace and metric providers for the
// duration of the test. It also initialises testtel instruments on the new
// meter provider and restores the previous global providers on cleanup.
func setupTestProviders(t *testing.T) (*tracetest.InMemoryExporter, *sdkmetric.ManualReader) {
	t.Helper()

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prevTP := otel.GetTracerProvider()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prevMP := otel.GetMeterProvider()

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	if err := testtel.InitInstrumentsForTest(); err != nil {
		t.Fatalf("InitInstrumentsForTest: %v", err)
	}

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
	})

	return exp, reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return rm
}

func findHistogram(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
	t.Helper()
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				h, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("%s data type = %T, want Histogram[float64]", name, m.Data)
				}
				return h
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Histogram[float64]{}
}

func histogramHasPoint(h metricdata.Histogram[float64], wantAttrs map[string]string, minCount uint64) bool {
	for _, pt := range h.DataPoints {
		if pt.Count < minCount {
			continue
		}
		match := true
		for k, v := range wantAttrs {
			got, ok := pt.Attributes.Value(attribute.Key(k))
			if !ok || got.AsString() != v {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// ---- Init ----

func TestInit_NoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := testtel.Init(context.Background())
	if err != nil {
		t.Fatalf("Init() with no endpoint error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init() returned nil shutdown function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown() error = %v", err)
	}
}

// ---- Start ----

func TestStart_ReturnsValidContext(t *testing.T) {
	exp, _ := setupTestProviders(t)
	_ = exp

	ctx := testtel.Start(t)
	if ctx == nil {
		t.Fatal("Start() returned nil context")
	}
}

func TestStart_SpanEndsOnCleanup(t *testing.T) {
	exp, _ := setupTestProviders(t)

	var innerT *testing.T
	t.Run("inner", func(t *testing.T) {
		innerT = t
		_ = testtel.Start(t)
		// Span is not ended yet – cleanup has not run.
		if len(exp.GetSpans()) != 0 {
			t.Errorf("span ended before test cleanup: got %d spans", len(exp.GetSpans()))
		}
	})
	// After t.Run returns, inner test's cleanup has run → span was ended.
	_ = innerT
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span after subtest, got %d", len(spans))
	}
	if spans[0].Name != "TestStart_SpanEndsOnCleanup/inner" {
		t.Errorf("span name = %q", spans[0].Name)
	}
}

func TestStart_RecordsDurationMetric(t *testing.T) {
	_, reader := setupTestProviders(t)

	t.Run("timed", func(t *testing.T) {
		_ = testtel.Start(t)
	})

	rm := collectMetrics(t, reader)
	h := findHistogram(t, rm, "make_mcp_test_duration_seconds")
	want := map[string]string{
		"test.name":   "TestStart_RecordsDurationMetric/timed",
		"test.status": "pass",
	}
	if !histogramHasPoint(h, want, 1) {
		t.Fatalf("duration metric missing expected point; data=%#v", h.DataPoints)
	}
}

// TestStart_PassingSpanHasOkStatus verifies that a passing test span has Ok
// status and no "test.failed" events attached.
func TestStart_PassingSpanHasOkStatus(t *testing.T) {
	exp, _ := setupTestProviders(t)

	t.Run("passing", func(t *testing.T) {
		_ = testtel.Start(t)
	})

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status.Code.String() != "Ok" {
		t.Errorf("span status = %q, want Ok for passing test", spans[0].Status.Code)
	}
	for _, ev := range spans[0].Events {
		if ev.Name == "test.failed" {
			t.Errorf("unexpected test.failed event on passing span")
		}
	}
}

func TestStart_SpanLinkFromTraceparent(t *testing.T) {
	// Set a valid W3C traceparent so externalSpanContext returns a valid context.
	t.Setenv("TRACEPARENT", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	exp, _ := setupTestProviders(t)

	t.Run("linked", func(t *testing.T) {
		_ = testtel.Start(t)
	})

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if len(spans[0].Links) != 1 {
		t.Fatalf("expected 1 span link (to TRACEPARENT), got %d", len(spans[0].Links))
	}
	wantTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	if got := spans[0].Links[0].SpanContext.TraceID().String(); got != wantTraceID {
		t.Errorf("link trace ID = %q, want %q", got, wantTraceID)
	}
}

func TestStart_NoLinkWhenTraceparentAbsent(t *testing.T) {
	t.Setenv("TRACEPARENT", "")
	exp, _ := setupTestProviders(t)

	t.Run("unlinked", func(t *testing.T) {
		_ = testtel.Start(t)
	})

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if len(spans[0].Links) != 0 {
		t.Errorf("expected 0 span links when TRACEPARENT is unset, got %d", len(spans[0].Links))
	}
}

// ---- Benchmark ----

func BenchmarkStart(b *testing.B) {
	ctx := testtel.Start(b)
	_ = ctx
	b.ResetTimer()
	for b.Loop() {
		_ = testtel.Start(b)
	}
}
