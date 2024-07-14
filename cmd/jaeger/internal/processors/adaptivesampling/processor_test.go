// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

type samplingHost struct {
	t                 *testing.T
	samplingExtension component.Component
}

func (host samplingHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		remotesampling.ID: host.samplingExtension,
	}
}

func (host samplingHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (samplingHost) GetFactory(_ component.Kind, _ component.Type) component.Factory { return nil }
func (samplingHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

func makeStorageExtension(t *testing.T, memstoreName string) component.Host {
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
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

func makeRemoteSamplingExtension(t *testing.T, cfg component.Config) samplingHost {
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
	host := samplingHost{t: t, samplingExtension: samplingExtension}
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
	}
	config, ok := createDefaultConfig().(*Config)
	require.True(t, ok)
	traceProcessor := newTraceProcessor(*config, telemetrySettings)

	host := makeRemoteSamplingExtension(t, &remotesampling.Config{
		Adaptive: &remotesampling.AdaptiveConfig{
			SamplingStore: "foobar",
			Options:       adaptive.DefaultOptions(),
		},
	})

	err := traceProcessor.start(context.Background(), host)
	require.NoError(t, err)

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
