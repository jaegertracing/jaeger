// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/nopexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/receiver"
	colservice "go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/extensions"
	"go.opentelemetry.io/collector/service/pipelines"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
)

func TestFilteringTracerProvider_AllowedComponent(t *testing.T) {
	realTP := sdktrace.NewTracerProvider()
	defer realTP.Shutdown(context.Background())
	noop := nooptrace.NewTracerProvider()

	ftp := &filteringTracerProvider{real: realTP, noop: noop}

	for _, id := range []string{jaegerquery.ID.String(), jaegermcp.ID.String()} {
		t.Run(id, func(t *testing.T) {
			tr := ftp.Tracer("test", trace.WithInstrumentationAttributes(
				attribute.String("otelcol.component.id", id),
			))
			_, span := tr.Start(context.Background(), "op")
			assert.True(t, span.SpanContext().IsValid(),
				"allowlisted component %q must get a real (recording) tracer", id)
			span.End()
		})
	}
}

func TestFilteringTracerProvider_BlockedComponents(t *testing.T) {
	realTP := sdktrace.NewTracerProvider()
	defer realTP.Shutdown(context.Background())
	noop := nooptrace.NewTracerProvider()

	ftp := &filteringTracerProvider{real: realTP, noop: noop}

	for _, id := range []string{"otlp", "jaeger", "batch", "", "unknown_ext"} {
		t.Run(id, func(t *testing.T) {
			tr := ftp.Tracer("test", trace.WithInstrumentationAttributes(
				attribute.String("otelcol.component.id", id),
			))
			_, span := tr.Start(context.Background(), "op")
			assert.False(t, span.SpanContext().IsValid(),
				"non-allowlisted component %q must get noop tracer", id)
			span.End()
		})
	}
}

func TestFilteringTracerProvider_NoAttribute(t *testing.T) {
	realTP := sdktrace.NewTracerProvider()
	defer realTP.Shutdown(context.Background())
	noop := nooptrace.NewTracerProvider()

	ftp := &filteringTracerProvider{real: realTP, noop: noop}

	// No instrumentation attributes at all → noop
	tr := ftp.Tracer("test")
	_, span := tr.Start(context.Background(), "op")
	assert.False(t, span.SpanContext().IsValid())
	span.End()
}

func TestFilteringTracerProvider_Shutdown(t *testing.T) {
	realTP := sdktrace.NewTracerProvider()
	noop := nooptrace.NewTracerProvider()

	closed := false
	closer := func(_ context.Context) error {
		closed = true
		return nil
	}

	ftp := &filteringTracerProvider{real: realTP, noop: noop, closer: closer}
	require.NoError(t, ftp.Shutdown(context.Background()))
	assert.True(t, closed)
}

func TestFilteringTracerProvider_ShutdownNilCloser(t *testing.T) {
	realTP := sdktrace.NewTracerProvider()
	defer realTP.Shutdown(context.Background())
	noop := nooptrace.NewTracerProvider()

	ftp := &filteringTracerProvider{real: realTP, noop: noop}
	require.NoError(t, ftp.Shutdown(context.Background()))
}

// TestFilteringTracerProvider_FrameworkInjection validates that the collector
// framework's componentattribute.tracerProviderWithAttributes wrapper injects
// the otelcol.component.id attribute into every Tracer() call. If upstream
// changes this behavior, this test fails at the next dependency bump.
func TestFilteringTracerProvider_FrameworkInjection(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")

	var receiverGotReal, extensionGotReal atomic.Bool

	// checkTP records whether the TracerProvider delivers a real (recording) span.
	checkTP := func(tp trace.TracerProvider, out *atomic.Bool) {
		_, span := tp.Tracer("probe").Start(context.Background(), "op")
		out.Store(span.SpanContext().IsValid())
		span.End()
	}

	// A receiver that records what kind of TracerProvider it received.
	recvType := component.MustNewType("test_recv")
	recvFactory := receiver.NewFactory(
		recvType,
		func() component.Config { return &struct{}{} },
		receiver.WithTraces(
			func(_ context.Context, set receiver.Settings, _ component.Config, _ consumer.Traces) (receiver.Traces, error) {
				checkTP(set.TracerProvider, &receiverGotReal)
				return &nopReceiver{}, nil
			},
			component.StabilityLevelDevelopment,
		),
	)

	// An extension with component ID matching jaeger_query that does the same.
	extType := jaegerquery.ID.Type()
	extFactory := extension.NewFactory(
		extType,
		func() component.Config { return &struct{}{} },
		func(_ context.Context, set extension.Settings, _ component.Config) (extension.Extension, error) {
			checkTP(set.TracerProvider, &extensionGotReal)
			return &nopExtension{}, nil
		},
		component.StabilityLevelDevelopment,
	)

	nopExp := nopexporter.NewFactory()

	set := colservice.Settings{
		BuildInfo:     component.NewDefaultBuildInfo(),
		CollectorConf: confmap.New(),
		ReceiversConfigs: map[component.ID]component.Config{
			component.NewID(recvType): &struct{}{},
		},
		ReceiversFactories: map[component.Type]receiver.Factory{
			recvType: recvFactory,
		},
		ExportersConfigs: map[component.ID]component.Config{
			component.NewID(nopExp.Type()): nopExp.CreateDefaultConfig(),
		},
		ExportersFactories: map[component.Type]exporter.Factory{
			nopExp.Type(): nopExp,
		},
		ExtensionsConfigs: map[component.ID]component.Config{
			component.NewID(extType): &struct{}{},
		},
		ExtensionsFactories: map[component.Type]extension.Factory{
			extType: extFactory,
		},
		AsyncErrorChannel: make(chan error),
		TelemetryFactory:  WrapFactory(otelconftelemetry.NewFactory()),
	}

	// Use the default telemetry config but disable the Prometheus metrics exporter
	// to avoid port-8888 conflicts when running tests in parallel.
	telCfg := otelconftelemetry.NewFactory().CreateDefaultConfig().(*otelconftelemetry.Config)
	telCfg.Metrics.Level = configtelemetry.LevelNone

	cfg := colservice.Config{
		Telemetry:  telCfg,
		Extensions: extensions.Config{component.NewID(extType)},
		Pipelines: pipelines.Config{
			pipeline.NewID(pipeline.SignalTraces): {
				Receivers: []component.ID{component.NewID(recvType)},
				Exporters: []component.ID{component.NewID(nopExp.Type())},
			},
		},
	}

	ctx := context.Background()
	srv, err := colservice.New(ctx, set, cfg)
	require.NoError(t, err)
	require.NoError(t, srv.Start(ctx))
	t.Cleanup(func() { require.NoError(t, srv.Shutdown(ctx)) })

	assert.False(t, receiverGotReal.Load(), "receiver must NOT get real TracerProvider")
	assert.True(t, extensionGotReal.Load(), "jaeger_query extension must get real TracerProvider")
}

type nopReceiver struct{}

func (*nopReceiver) Start(context.Context, component.Host) error { return nil }
func (*nopReceiver) Shutdown(context.Context) error              { return nil }

type nopExtension struct{}

func (*nopExtension) Start(context.Context, component.Host) error { return nil }
func (*nopExtension) Shutdown(context.Context) error              { return nil }
