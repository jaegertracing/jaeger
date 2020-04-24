// Copyright (c) 2020 The Jaeger Authors.
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

package cassandra

import (
	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/uber/jaeger-lib/metrics"

	storageOtelExporter "github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
)

// New creates Cassandra exporter/storage
func New(config *Config, params component.ExporterCreateParams) (component.TraceExporter, error) {
	f := cassandra.NewFactory()
	f.InitFromOptions(&config.Options)

	err := f.Initialize(metrics.NullFactory, params.Logger)
	if err != nil {
		return nil, err
	}
	return storageOtelExporter.NewSpanWriterExporter(config, f)
}
