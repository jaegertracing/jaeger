package telemetery

import (
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/plugin/metrics"
	"go.uber.org/zap"
)

type Setting struct {
	Logger        *zap.Logger
	Tracer        *jtracer.JTracer
	MeterProvider metrics.Factory
	HC            *healthcheck.HealthCheck
}
