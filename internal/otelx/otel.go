package otelx

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Setup configures a global TracerProvider. If endpoint is empty or OTEL_SDK_DISABLED=true,
// a no-op provider is installed (safe for unit tests / local runs without Jaeger).
func Setup(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if disabled := strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true"); disabled || strings.TrimSpace(endpoint) == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.TraceContext{})
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithInsecure()}
	ep := strings.TrimSpace(endpoint)
	if strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://") {
		opts = append(opts, otlptracehttp.WithEndpointURL(strings.TrimRight(ep, "/")))
	} else {
		opts = append(opts, otlptracehttp.WithEndpoint(strings.TrimRight(ep, "/")))
	}
	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns the named tracer from the global provider.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
