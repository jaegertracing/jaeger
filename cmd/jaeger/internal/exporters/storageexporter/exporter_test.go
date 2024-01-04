// Copyright (c) 2018 The Jaeger Authors.
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

	yaml "gopkg.in/yaml.v2"

	"github.com/crossdock/crossdock-go/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
)

var storage_exporter *storageExporter
var host component.Host

var yamlConfig = `
exporters:
  jaeger_storage_exporter:
    trace_storage: memstore

extensions:
  jaeger_storage:
    memory:
      memstore:
        max_traces: 100000
`

func TestExporter(t *testing.T) {
	var config component.Config
	yaml.Unmarshal([]byte(yamlConfig), &config)
	cfg := config.(*Config)
	telemetry_settings := componenttest.NewNopTelemetrySettings()
	storage_exporter = newExporter(cfg, telemetry_settings)
	assert.Equal(t, storage_exporter.logger, telemetry_settings.Logger)
	assert.Equal(t, storage_exporter.config, cfg)

	host = componenttest.NewNopHost()
	err := storage_exporter.start(context.Background(), host)
	fmt.Println(err)
	assert.Nil(t, storage_exporter.spanWriter)
	assert.Contains(t, err.Error(), "cannot find storage factory")

	err = storage_exporter.close(context.Background())
	assert.Nil(t, err)
}
