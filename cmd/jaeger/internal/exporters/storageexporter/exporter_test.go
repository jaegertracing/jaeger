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
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	factoryMocks "github.com/jaegertracing/jaeger/internal/storage/v1/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

type mockStorageExt struct {
	name           string
	factory        *factoryMocks.Factory
	metricsFactory *factoryMocks.MetricStoreFactory
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

func (m *mockStorageExt) MetricStorageFactory(name string) (storage.MetricStoreFactory, bool) {
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

	exp := &storageExporter{
		config: &Config{
			TraceStorage: "bar",
		},
	}
	err := exp.start(context.Background(), host)
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

	exp := &storageExporter{
		config: &Config{
			TraceStorage: "foo",
		},
	}
	err := exp.start(context.Background(), host)
	require.ErrorContains(t, err, "mocked error")
}

func TestExporter(t *testing.T) {
	exporterFactory := NewFactory()

	ctx := context.Background()
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
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

	storageFactory, err := jaegerstorage.GetTraceStoreFactory(memstoreName, host)
	require.NoError(t, err)
	traceReader, err := storageFactory.CreateTraceReader()
	require.NoError(t, err)
	requiredTraceID := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	getTracesIter := traceReader.GetTraces(ctx, tracestore.GetTraceParams{
		TraceID: requiredTraceID,
	})
	requiredTrace, err := iter.FlattenWithErrors(getTracesIter)
	require.NoError(t, err)
	resource := requiredTrace[0].ResourceSpans().At(0)
	assert.Equal(t, spanID, resource.ScopeSpans().At(0).Spans().At(0).SpanID())

	// check that the service name attribute was added by the sanitizer
	serviceName, ok := resource.Resource().Attributes().Get(string(otelsemconv.ServiceNameKey))
	require.True(t, ok)
	require.Equal(t, "missing-service-name", serviceName.Str())
}

func makeStorageExtension(t *testing.T, memstoreName string) component.Host {
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}
	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.Create(
		context.Background(),
		extension.Settings{
			TelemetrySettings: telemetrySettings,
		},
		&jaegerstorage.Config{
			TraceBackends: map[string]jaegerstorage.TraceBackend{
				memstoreName: {Memory: &memory.Configuration{MaxTraces: 10000}},
			},
		},
	)
	require.NoError(t, err)

	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, storageExtension)

	err = storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(context.Background())) })

	return host
}
