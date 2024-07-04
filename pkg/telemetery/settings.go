package telemetery

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

type Setting struct {
	Logger         *zap.Logger
	TracerProvider trace.TracerProvider
	Metrics        metrics.Factory
	ReportStatus   func(*component.StatusEvent)
}

func InitTracerProvider(serviceName string) (*sdkTrace.TracerProvider, error) {
	ctx := context.Background()
	traceExporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	bsp := sdkTrace.NewBatchSpanProcessor(traceExporter)
	res, err := resource.New(
		ctx,
		resource.WithSchemaURL(otelsemconv.SchemaURL),
		resource.WithAttributes(otelsemconv.ServiceNameKey.String(serviceName)),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOSType(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, err
	}
	traceProvider := sdkTrace.NewTracerProvider(
		sdkTrace.WithSpanProcessor(bsp),
		sdkTrace.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)
	return traceProvider, nil
}

func HCAdapter(hc *healthcheck.HealthCheck) func(*component.StatusEvent) {
	return func(event *component.StatusEvent) {
		var hcStatus healthcheck.Status
		switch event.Status() {
		case component.StatusOK:
			hcStatus = healthcheck.Ready
		case component.StatusStarting,
			component.StatusRecoverableError,
			component.StatusPermanentError,
			component.StatusNone,
			component.StatusStopping:
			hcStatus = healthcheck.Unavailable
		case component.StatusFatalError, component.StatusStopped:
			hcStatus = healthcheck.Broken
		}
		hc.Set(hcStatus)
	}
}
