// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"

	"go.opentelemetry.io/collector/component"
	coltelemetry "go.opentelemetry.io/collector/service/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/internal/jtracer"
)

// tracedComponents is the allowlist of otelcol.component.id values that receive
// the real TracerProvider. All other components (receivers, processors, exporters,
// connectors, and unlisted extensions) get a noop tracer, preventing recursive
// self-tracing loops when Jaeger's OTLP receiver is the export destination.
var tracedComponents = map[string]struct{}{
	jaegerquery.ID.String(): {},
	jaegermcp.ID.String():   {},
}

var componentIDKey = attribute.Key("otelcol.component.id")

// filteringTracerProvider wraps a real trace.TracerProvider and returns a noop
// tracer for all components not in the tracedComponents allowlist.
//
// It embeds embedded.TracerProvider to satisfy the unexported marker method
// required by coltelemetry.TracerProvider (same pattern as the collector's own
// noopNoContextTracerProvider).
type filteringTracerProvider struct {
	embedded.TracerProvider
	real   trace.TracerProvider
	noop   trace.TracerProvider
	closer func(context.Context) error
}

var _ coltelemetry.TracerProvider = (*filteringTracerProvider)(nil)

// Tracer implements trace.TracerProvider. It inspects the otelcol.component.id
// instrumentation attribute injected by the collector framework and routes to
// the real provider only for allowlisted components.
func (f *filteringTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	cfg := trace.NewTracerConfig(opts...)
	attrs := cfg.InstrumentationAttributes()
	id, _ := attrs.Value(componentIDKey)
	if _, ok := tracedComponents[id.AsString()]; ok {
		return f.real.Tracer(name, opts...)
	}
	return f.noop.Tracer(name, opts...)
}

// Shutdown flushes and shuts down the underlying real TracerProvider and any
// associated background goroutines (e.g. the jaeger_remote sampler poller).
func (f *filteringTracerProvider) Shutdown(ctx context.Context) error {
	if f.closer != nil {
		return f.closer(ctx)
	}
	return nil
}

// WrapFactory returns a telemetry.Factory that delegates everything to the given
// factory except CreateTracerProvider, which is replaced with Jaeger's filtering
// implementation. Use it in place of otelconftelemetry.NewFactory() in components.go.
func WrapFactory(delegate coltelemetry.Factory) coltelemetry.Factory {
	return coltelemetry.NewFactory(
		delegate.CreateDefaultConfig,
		coltelemetry.WithCreateResource(delegate.CreateResource),
		coltelemetry.WithCreateLogger(delegate.CreateLogger),
		coltelemetry.WithCreateMeterProvider(delegate.CreateMeterProvider),
		coltelemetry.WithCreateTracerProvider(createTracerProvider),
	)
}

// createTracerProvider is the CreateTracerProviderFunc used by WrapFactory.
// It creates one real trace.TracerProvider via jtracer and wraps it in a
// filteringTracerProvider so only allowlisted components see real traces.
func createTracerProvider(ctx context.Context, _ coltelemetry.TracerSettings, _ component.Config) (coltelemetry.TracerProvider, error) {
	realTP, closer, err := jtracer.NewProvider(ctx, "jaeger")
	if err != nil {
		return nil, err
	}
	return &filteringTracerProvider{
		real:   realTP,
		noop:   nooptrace.NewTracerProvider(),
		closer: closer,
	}, nil
}
