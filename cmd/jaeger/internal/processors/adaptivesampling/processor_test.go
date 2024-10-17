// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"context"
	"errors"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

func makeStorageExtension(t *testing.T, memstoreName string) component.Host {
	t.Helper()
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
			return noopmetric.NewMeterProvider()
		},
		MeterProvider: noopmetric.NewMeterProvider(),
	}
	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.CreateExtension(
		context.Background(),
		extension.Settings{
			TelemetrySettings: telemetrySettings,
		},
		&jaegerstorage.Config{Backends: map[string]jaegerstorage.Backend{
			memstoreName: {Memory: &memory.Configuration{MaxTraces: 10000}},
		}},
	)
	require.NoError(t, err)

	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, storageExtension)

	err = storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(context.Background())) })
	return host
}

var _ component.Config = (*Config)(nil)

func makeRemoteSamplingExtension(t *testing.T, cfg component.Config) component.Host {
	t.Helper()
	extensionFactory := remotesampling.NewFactory()
	samplingExtension, err := extensionFactory.CreateExtension(
		context.Background(),
		extension.Settings{
			TelemetrySettings: component.TelemetrySettings{
				Logger:         zap.L(),
				TracerProvider: nooptrace.NewTracerProvider(),
			},
		},
		cfg,
	)
	require.NoError(t, err)
	host := storagetest.NewStorageHost().WithExtension(remotesampling.ID, samplingExtension)
	storageHost := makeStorageExtension(t, "foobar")

	err = samplingExtension.Start(context.Background(), storageHost)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, samplingExtension.Shutdown(context.Background())) })
	return host
}

func TestNewTraceProcessor(t *testing.T) {
	telemetrySettings := component.TelemetrySettings{
		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
	}
	config, ok := createDefaultConfig().(*Config)
	require.True(t, ok)
	newTraceProcessor := newTraceProcessor(*config, telemetrySettings)
	require.NotNil(t, newTraceProcessor)
}

func TestTraceProcessor(t *testing.T) {
	telemetrySettings := component.TelemetrySettings{
		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
		LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
			return noopmetric.NewMeterProvider()
		},
		MeterProvider: noopmetric.NewMeterProvider(),
	}
	config := createDefaultConfig().(*Config)
	traceProcessor := newTraceProcessor(*config, telemetrySettings)

	rsCfg := &remotesampling.Config{
		Adaptive: &remotesampling.AdaptiveConfig{
			SamplingStore: "foobar",
			Options:       adaptive.DefaultOptions(),
		},
	}
	host := makeRemoteSamplingExtension(t, rsCfg)

	rsCfg.Adaptive.Options.AggregationBuckets = 0
	err := traceProcessor.start(context.Background(), host)
	require.ErrorContains(t, err, "AggregationBuckets must be greater than 0")

	rsCfg.Adaptive.Options = adaptive.DefaultOptions()
	require.NoError(t, traceProcessor.start(context.Background(), host))

	twww := makeTracesOneSpan()
	trace, err := traceProcessor.processTraces(context.Background(), twww)
	require.NoError(t, err)
	require.NotNil(t, trace)

	err = traceProcessor.close(context.Background())
	require.NoError(t, err)
}

func makeTracesOneSpan() ptrace.Traces {
	traces := ptrace.NewTraces()
	rSpans := traces.ResourceSpans().AppendEmpty()
	sSpans := rSpans.ScopeSpans().AppendEmpty()
	span := sSpans.Spans().AppendEmpty()
	span.SetName("test")
	return traces
}

func TestGetAdaptiveSamplingComponentsError(t *testing.T) {
	processor := &traceProcessor{}
	err := processor.start(context.Background(), storagetest.NewStorageHost())
	require.ErrorContains(t, err, "cannot load adaptive sampling components")
}

// aggregator that returns error from Close()
type notClosingAgg struct{}

func (*notClosingAgg) Close() error { return errors.New("not closing") }

func (*notClosingAgg) HandleRootSpan(*model.Span, *zap.Logger)                     {}
func (*notClosingAgg) RecordThroughput(string, string, model.SamplerType, float64) {}
func (*notClosingAgg) Start()                                                      {}

func TestTraceProcessorCloseError(t *testing.T) {
	processor := &traceProcessor{
		aggregator: &notClosingAgg{},
	}
	require.ErrorContains(t, processor.close(context.Background()), "not closing")
}
