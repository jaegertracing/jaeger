// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"errors"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
	factoryMocks "github.com/jaegertracing/jaeger/storage/mocks"
)

type mockStorageExt struct {
	name           string
	factory        *factoryMocks.Factory
	metricsFactory *factoryMocks.MetricsFactory
}

var _ jaegerstorage.Extension = (*mockStorageExt)(nil)

func (*mockStorageExt) Start(context.Context, component.Host) error {
	panic("not implemented")
}

func (*mockStorageExt) Shutdown(context.Context) error {
	panic("not implemented")
}

func (m *mockStorageExt) TraceStorageFactory(name string) (storage.Factory, bool) {
	if m.name == name {
		return m.factory, true
	}
	return nil, false
}

func (m *mockStorageExt) MetricStorageFactory(name string) (storage.MetricsFactory, bool) {
	if m.name == name {
		return m.metricsFactory, true
	}
	return nil, false
}

func TestExporterConfigError(t *testing.T) {
	config := createDefaultConfig().(*Config)
	err := config.Validate()
	require.EqualError(t, err, "TraceStorage: non zero value required")
}

func TestExporterStartBadNameError(t *testing.T) {
	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, &mockStorageExt{name: "foo"})

	storageExporter := &storageExporter{
		config: &Config{
			TraceStorage: "bar",
		},
	}
	err := storageExporter.start(context.Background(), host)
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot find storage factory")
}

func TestExporterStartBadSpanstoreError(t *testing.T) {
	factory := new(factoryMocks.Factory)
	factory.On("CreateSpanWriter").Return(nil, errors.New("mocked error"))

	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, &mockStorageExt{
		name:    "foo",
		factory: factory,
	})

	storageExporter := &storageExporter{
		config: &Config{
			TraceStorage: "foo",
		},
	}
	err := storageExporter.start(context.Background(), host)
	require.Error(t, err)
	require.ErrorContains(t, err, "mocked error")
}

func TestExporter(t *testing.T) {
	exporterFactory := NewFactory()

	ctx := context.Background()
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
			return noopmetric.NewMeterProvider()
		},
		MeterProvider: noopmetric.NewMeterProvider(),
	}

	const memstoreName = "memstore"
	config := &Config{
		TraceStorage: memstoreName,
	}
	err := config.Validate()
	require.NoError(t, err)

	tracesExporter, err := exporterFactory.CreateTraces(ctx, exporter.Settings{
		ID:                ID,
		TelemetrySettings: telemetrySettings,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}, config)
	require.NoError(t, err)

	host := makeStorageExtension(t, memstoreName)

	require.NoError(t, tracesExporter.Start(ctx, host))
	defer func() {
		require.NoError(t, tracesExporter.Shutdown(ctx))
	}()

	traces := ptrace.NewTraces()
	rSpans := traces.ResourceSpans().AppendEmpty()
	sSpans := rSpans.ScopeSpans().AppendEmpty()
	span := sSpans.Spans().AppendEmpty()

	spanID := pcommon.NewSpanIDEmpty()
	spanID[5] = 5 // 0000000000050000
	span.SetSpanID(spanID)

	traceID := pcommon.NewTraceIDEmpty()
	traceID[15] = 1 // 00000000000000000000000000000001
	span.SetTraceID(traceID)

	err = tracesExporter.ConsumeTraces(ctx, traces)
	require.NoError(t, err)

	storageFactory, err := jaegerstorage.GetStorageFactory(memstoreName, host)
	require.NoError(t, err)
	spanReader, err := storageFactory.CreateSpanReader()
	require.NoError(t, err)
	requiredTraceID := model.NewTraceID(0, 1) // 00000000000000000000000000000001
	requiredTrace, err := spanReader.GetTrace(ctx, requiredTraceID)
	require.NoError(t, err)
	assert.Equal(t, spanID.String(), requiredTrace.Spans[0].SpanID.String())

	// check that the service name attribute was added by the sanitizer
	require.Equal(t, "missing-service-name", requiredTrace.Spans[0].Process.ServiceName)
}

func makeStorageExtension(t *testing.T, memstoreName string) component.Host {
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
			return noopmetric.NewMeterProvider()
		},
		MeterProvider: noopmetric.NewMeterProvider(),
	}
	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.Create(
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
