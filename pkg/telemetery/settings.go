package telemetery

import (
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type Setting struct {
	Logger        *zap.Logger
	Tracer        *jtracer.JTracer
	MeterProvider metric.MeterProvider
}
