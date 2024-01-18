// Copyright (c) 2024 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storageexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/model"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
)

type storageHost struct {
	t                *testing.T
	storageExtension component.Component
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	myMap := make(map[component.ID]component.Component)
	myMap[jaegerstorage.ID] = host.storageExtension
	return myMap
}

func (host storageHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	return nil
}

func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

func TestExporterConfigError(t *testing.T) {
	config := createDefaultConfig().(*Config)
	err := config.Validate()
	require.EqualError(t, err, "TraceStorage: non zero value required")
}

func TestExporterStartError(t *testing.T) {
	host := makeStorageExtension(t, "foo")
	exporter := &storageExporter{
		config: &Config{
			TraceStorage: "bar",
		},
	}
	err := exporter.start(context.Background(), host)
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot find storage factory")
}

func TestExporter(t *testing.T) {
	exporterFactory := NewFactory()

	ctx := context.Background()
	telemetrySettings := component.TelemetrySettings{
		Logger:         zap.L(),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}

	const memstoreName = "memstore"
	config := &Config{
		TraceStorage: memstoreName,
	}
	err := config.Validate()
	require.NoError(t, err)

	tracesExporter, err := exporterFactory.CreateTracesExporter(ctx, exporter.CreateSettings{
		ID:                ID,
		TelemetrySettings: telemetrySettings,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}, config)
	require.NoError(t, err)

	host := makeStorageExtension(t, memstoreName)

	err = tracesExporter.Start(ctx, host)
	require.NoError(t, err)
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
}

func makeStorageExtension(t *testing.T, memstoreName string) storageHost {
	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.CreateExtension(
		context.Background(),
		extension.CreateSettings{
			TelemetrySettings: component.TelemetrySettings{
				Logger:         zap.L(),
				TracerProvider: nooptrace.NewTracerProvider(),
			},
		},
		&jaegerstorage.Config{Memory: map[string]memoryCfg.Configuration{
			memstoreName: {MaxTraces: 10000},
		}})
	require.NoError(t, err)
	host := storageHost{t: t, storageExtension: storageExtension}

	err = storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(context.Background())) })
	return host
}
