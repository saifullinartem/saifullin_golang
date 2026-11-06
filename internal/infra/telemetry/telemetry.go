package telemetry

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/saifullinartem/saifullin_golang/internal/config"
)

type Providers struct {
	Traces  *sdktrace.TracerProvider
	Metrics *sdkmetric.MeterProvider
}

func New(ctx context.Context, cfg config.Config) (*Providers, error) {
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithAttributes(attribute.String("service.name", cfg.OTelServiceName), attribute.String("service.version", cfg.ServiceVersion)))
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}
	var te sdktrace.SpanExporter
	var me sdkmetric.Exporter
	if cfg.OTelProtocol == "grpc" {
		te, err = otlptracegrpc.New(ctx)
	} else {
		te, err = otlptracehttp.New(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}
	if cfg.OTelProtocol == "grpc" {
		me, err = otlpmetricgrpc.New(ctx)
	} else {
		me, err = otlpmetrichttp.New(ctx)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("metric exporter: %w", err), te.Shutdown(ctx))
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(te))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(me)))
	return &Providers{tp, mp}, nil
}

func (p *Providers) Shutdown(ctx context.Context) error {
	return errors.Join(p.Traces.Shutdown(ctx), p.Metrics.Shutdown(ctx))
}
