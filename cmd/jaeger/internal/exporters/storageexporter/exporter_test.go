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

	"github.com/crossdock/crossdock-go/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
)

var (
	storage_exporter *storageExporter
	host             component.Host
)

func TestExporter(t *testing.T) {
	config := &Config{}
	config.TraceStorage = "memstore"
	telemetry_settings := componenttest.NewNopTelemetrySettings()
	storage_exporter = newExporter(config, telemetry_settings)
	assert.Equal(t, storage_exporter.logger, telemetry_settings.Logger)
	assert.Equal(t, storage_exporter.config, config)

	host = componenttest.NewNopHost()
	err := storage_exporter.start(context.Background(), host)
	assert.Nil(t, storage_exporter.spanWriter)
	assert.Contains(t, err.Error(), "cannot find storage factory")

	err = storage_exporter.close(context.Background())
	assert.Nil(t, err)
}
