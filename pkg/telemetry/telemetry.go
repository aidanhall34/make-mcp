package telemetry

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	promclient "github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init configures OpenTelemetry from environment variables and returns a shutdown function.
func Init(ctx context.Context) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("make-mcp"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	shutdownFns := []func(context.Context) error{}

	if err := initTraceProvider(ctx, res, &shutdownFns); err != nil {
		return nil, err
	}
	if err := initMetricProvider(ctx, res, &shutdownFns); err != nil {
		return nil, err
	}
	if err := initMetrics(); err != nil {
		return nil, fmt.Errorf("init metrics: %w", err)
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

func initTraceProvider(ctx context.Context, res *resource.Resource, shutdownFns *[]func(context.Context) error) error {
	exporter := strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER"))
	if exporter == "" || exporter == "none" {
		return nil
	}
	if exporter != "otlp" {
		return fmt.Errorf("unsupported OTEL_TRACES_EXPORTER %q", exporter)
	}

	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	*shutdownFns = append(*shutdownFns, provider.Shutdown)
	return nil
}

func initMetricProvider(ctx context.Context, res *resource.Resource, shutdownFns *[]func(context.Context) error) error {
	exporter := strings.TrimSpace(os.Getenv("OTEL_METRICS_EXPORTER"))
	if exporter == "" {
		exporter = "prometheus"
	}

	switch exporter {
	case "none":
		return nil
	case "otlp":
		metricExporter, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithTemporalitySelector(metric.CumulativeTemporalitySelector),
		)
		if err != nil {
			return fmt.Errorf("create OTLP metric exporter: %w", err)
		}
		reader := metric.NewPeriodicReader(metricExporter)
		provider := metric.NewMeterProvider(
			metric.WithResource(res),
			metric.WithReader(reader),
		)
		otel.SetMeterProvider(provider)
		*shutdownFns = append(*shutdownFns, provider.Shutdown)
		return nil
	case "prometheus":
		promExporter, err := prometheus.New(prometheus.WithNamespace("make_mcp"))
		if err != nil {
			return fmt.Errorf("create prometheus exporter: %w", err)
		}
		provider := metric.NewMeterProvider(
			metric.WithResource(res),
			metric.WithReader(promExporter),
		)
		otel.SetMeterProvider(provider)
		*shutdownFns = append(*shutdownFns, provider.Shutdown)

		srv := &http.Server{
			Addr:    prometheusAddr(),
			Handler: promclient.Handler(),
		}
		go func() {
			_ = srv.ListenAndServe()
		}()
		*shutdownFns = append(*shutdownFns, srv.Shutdown)
		return nil
	default:
		return fmt.Errorf("unsupported OTEL_METRICS_EXPORTER %q", exporter)
	}
}

func prometheusAddr() string {
	host := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_PROMETHEUS_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_PROMETHEUS_PORT"))
	if port == "" {
		port = "9090"
	}
	return net.JoinHostPort(host, port)
}
