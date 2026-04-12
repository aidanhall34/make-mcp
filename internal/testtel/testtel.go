// Package testtel configures OpenTelemetry for test and benchmark binaries.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is set, Init creates real OTLP gRPC trace
// and metric providers pointing at that endpoint (default: http://localhost:4317
// when the dev LGTM stack is running). When the env var is absent, Init is a
// no-op so ordinary `go test` runs are unaffected.
//
// Usage – replace covercheck.Report in every TestMain:
//
//	func TestMain(m *testing.M) {
//	    os.Exit(testtel.RunMain(m, 0.95))
//	}
//
// Usage – instrument individual tests and benchmarks:
//
//	func TestFoo(t *testing.T) {
//	    ctx := testtel.Start(t) // creates root span; ctx carries it
//	    result, err := mycode.DoSomething(ctx, ...)
//	    ...
//	}
package testtel

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/internal/covercheck"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationScope = "github.com/aidanhall34/make-mcp/internal/testtel"

type instruments struct {
	testDurationSeconds metric.Float64Histogram
}

var (
	instMu sync.RWMutex
	inst   instruments
)

// Init initialises OTel trace and metric providers for the current test binary.
// If OTEL_EXPORTER_OTLP_ENDPOINT is empty, Init is a no-op: the global
// providers remain as no-ops, and a zero-cost shutdown function is returned.
//
// The trace and metric exporters use gRPC OTLP. The endpoint is read from
// OTEL_EXPORTER_OTLP_ENDPOINT (standard OTel env var).
func Init(ctx context.Context) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("make-mcp-tests")),
	)
	if err != nil {
		return nil, fmt.Errorf("testtel: build resource: %w", err)
	}

	var shutdownFns []func(context.Context) error

	traceExp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("testtel: create trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	shutdownFns = append(shutdownFns, tp.Shutdown)

	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithTemporalitySelector(sdkmetric.CumulativeTemporalitySelector),
	)
	if err != nil {
		return nil, fmt.Errorf("testtel: create metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
	)
	otel.SetMeterProvider(mp)
	shutdownFns = append(shutdownFns, mp.Shutdown)

	if err := initInstruments(); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		var firstErr error
		for i := len(shutdownFns) - 1; i >= 0; i-- {
			if err := shutdownFns[i](ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}, nil
}

// initInstruments creates the test metric instruments on the current global
// meter provider. Called by Init after the provider is registered, and by
// tests that set up their own in-memory providers.
func initInstruments() error {
	meter := otel.Meter(instrumentationScope)
	var (
		err error
		m   instruments
	)
	if m.testDurationSeconds, err = meter.Float64Histogram(
		"make_mcp_test_duration_seconds",
		metric.WithDescription("Duration of individual tests and benchmarks."),
		metric.WithUnit("s"),
	); err != nil {
		return fmt.Errorf("testtel: init test_duration_seconds: %w", err)
	}
	instMu.Lock()
	inst = m
	instMu.Unlock()
	return nil
}

func getInstruments() instruments {
	instMu.RLock()
	defer instMu.RUnlock()
	return inst
}

// RunMain is a drop-in replacement for covercheck.Report inside TestMain. It
// initialises test telemetry, runs the full test suite (including coverage
// enforcement), then flushes all pending spans and metrics before returning.
//
//	func TestMain(m *testing.M) {
//	    os.Exit(testtel.RunMain(m, 0.95))
//	}
func RunMain(m *testing.M, threshold float64) int {
	ctx := context.Background()
	shutdown, err := Init(ctx)
	if err != nil {
		// Telemetry init failures must not abort the test suite.
		fmt.Fprintf(os.Stderr, "testtel: init warning: %v\n", err)
		return covercheck.Report(m, threshold)
	}
	code := covercheck.Report(m, threshold)
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "testtel: shutdown warning: %v\n", err)
	}
	return code
}

// Start registers a root trace span for the test or benchmark and returns a
// context carrying that span. Passing this context to functions under test
// ensures that spans they create are children of this root, preserving full
// trace context propagation.
//
// The span ends and test duration is recorded as a histogram metric when tb
// completes (via t.Cleanup / b.Cleanup). On failure, a "test.failed" event is
// added to the span.
//
// The root span is linked to any external trace context present in the
// TRACEPARENT environment variable (W3C traceparent format). This allows CI/CD
// pipelines to correlate individual test traces with the executor's trace.
func Start(tb testing.TB) context.Context {
	tb.Helper()

	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
	}
	if lc := externalSpanContext(); lc.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: lc}))
	}

	ctx, span := otel.Tracer(instrumentationScope).Start(context.Background(), tb.Name(), opts...)
	start := time.Now()

	tb.Cleanup(func() {
		elapsed := time.Since(start)
		status := "pass"
		if tb.Failed() {
			status = "fail"
			span.AddEvent("test.failed",
				trace.WithAttributes(attribute.String("test.name", tb.Name())),
			)
			span.SetStatus(codes.Error, "test failed")
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()

		i := getInstruments()
		if i.testDurationSeconds != nil {
			i.testDurationSeconds.Record(ctx, elapsed.Seconds(),
				metric.WithAttributes(
					attribute.String("test.name", tb.Name()),
					attribute.String("test.status", status),
				),
			)
		}
	})

	return ctx
}

// externalSpanContext reads a W3C traceparent from the TRACEPARENT environment
// variable and returns the encoded SpanContext. Returns a zero SpanContext when
// the variable is absent or cannot be parsed.
func externalSpanContext() trace.SpanContext {
	tp := strings.TrimSpace(os.Getenv("TRACEPARENT"))
	if tp == "" {
		return trace.SpanContext{}
	}
	carrier := propagation.MapCarrier{"traceparent": tp}
	extracted := propagation.TraceContext{}.Extract(context.Background(), carrier)
	return trace.SpanContextFromContext(extracted)
}
