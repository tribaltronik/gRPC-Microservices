package otel

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

type OTel struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *metric.MeterProvider
	promExporter   *otelprom.Exporter
	promRegistry   *prometheus.Registry
}

func New(ctx context.Context, serviceName, otlpEndpoint string) (*OTel, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, err
	}

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)
	otel.SetTracerProvider(tracerProvider)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	promRegistry := prometheus.NewRegistry()

	promExporter, err := otelprom.New(otelprom.WithRegisterer(promRegistry))
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(promExporter),
	)
	otel.SetMeterProvider(meterProvider)

	return &OTel{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		promExporter:   promExporter,
		promRegistry:   promRegistry,
	}, nil
}

func (o *OTel) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(o.promRegistry, promhttp.HandlerOpts{})
}

func (o *OTel) Shutdown(ctx context.Context) error {
	if err := o.tracerProvider.Shutdown(ctx); err != nil {
		return err
	}
	if err := o.meterProvider.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}
