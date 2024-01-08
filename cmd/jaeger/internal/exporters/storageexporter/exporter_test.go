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
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/model"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
)

type storageHost struct {
	storageExtension *component.Component
	t                *testing.T
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	myMap := make(map[component.ID]component.Component)
	myMap[jaegerstorage.ID] = *host.storageExtension
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

func TestExporter(t *testing.T) {
	ctx := context.Background()
	const memstoreName = "memstore"
	config := &Config{}
	config.TraceStorage = memstoreName
	telemetrySettings := component.TelemetrySettings{
		Logger: zap.L(),
	}
	exporter := newExporter(config, telemetrySettings)
	assert.Equal(t, exporter.logger, telemetrySettings.Logger)
	assert.Equal(t, exporter.config, config)

	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.CreateExtension(
		ctx,
		extension.CreateSettings{
			TelemetrySettings: telemetrySettings,
		},
		&jaegerstorage.Config{Memory: map[string]memoryCfg.Configuration{
			memstoreName: {MaxTraces: 10000},
		}})
	host := storageHost{
		storageExtension: &storageExtension,
		t:                t,
	}
	require.NoError(t, err)

	storageExtension.Start(ctx, host)

	err = exporter.start(ctx, host)
	assert.NotNil(t, exporter.spanWriter)
	require.NoError(t, err)

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

	err = exporter.pushTraces(ctx, traces)
	require.NoError(t, err)

	exporterStorageFactory, err := jaegerstorage.GetStorageFactory(exporter.config.TraceStorage, host)
	require.NoError(t, err)
	exporterSpanReader, err := exporterStorageFactory.CreateSpanReader()
	require.NoError(t, err)
	requiredTraceID := model.NewTraceID(0, 1) // 00000000000000000000000000000001
	require.NoError(t, err)
	requiredTrace, err := exporterSpanReader.GetTrace(ctx, requiredTraceID)
	require.NoError(t, err)
	assert.Equal(t, spanID.String(), requiredTrace.Spans[0].SpanID.String())

	err = exporter.close(ctx)
	require.NoError(t, err)
}
