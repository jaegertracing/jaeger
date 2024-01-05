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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
)

var (
	storage_exporter *storageExporter
	host             component.Host
	storage_host     storageHost
)

type storageHost struct {
	component *component.Component
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	myMap := make(map[component.ID]component.Component)
	myMap[component.NewID("jaeger_storage")] = *host.component
	return myMap
}

func (storageHost) ReportFatalError(err error) {
	fmt.Println(err)
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	return nil
}

func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

func TestExporter(t *testing.T) {
	config := &Config{}
	config.TraceStorage = "memstore"
	telemetry_settings := componenttest.NewNopTelemetrySettings()
	storage_exporter = newExporter(config, telemetry_settings)
	assert.Equal(t, storage_exporter.logger, telemetry_settings.Logger)
	assert.Equal(t, storage_exporter.config, config)

	storage_factory := jaegerstorage.NewFactory()
	storage_config := jaegerstorage.Config{Memory: make(map[string]memoryCfg.Configuration)}
	storage_config.Memory["memstore"] = memoryCfg.Configuration{MaxTraces: 10000}
	storage_component, _ := storage_factory.CreateExtension(
		context.Background(),
		extension.CreateSettings{
			TelemetrySettings: telemetry_settings,
		},
		&storage_config)

	host = componenttest.NewNopHost()
	storage_component.Start(context.Background(), host)

	storage_host = storageHost{
		component: &storage_component,
	}
	err := storage_exporter.start(context.Background(), storage_host)
	assert.NotNil(t, storage_exporter.spanWriter)
	require.NoError(t, err)

	traces := ptrace.NewTraces()
	rSpans := traces.ResourceSpans().AppendEmpty()
	sSpans := rSpans.ScopeSpans().AppendEmpty()
	span := sSpans.Spans().AppendEmpty()
	span.SetName("test")

	err = storage_exporter.pushTraces(context.Background(), traces)
	require.NoError(t, err)

	err = storage_exporter.close(context.Background())
	require.NoError(t, err)
}
