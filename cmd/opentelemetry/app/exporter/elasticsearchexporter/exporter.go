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

package elasticsearchexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/jaegertracing/jaeger/plugin/storage/es"
)

// newExporter creates Elasticsearch exporter/storage.
func newExporter(ctx context.Context, config *Config, params component.ExporterCreateParams) (component.TracesExporter, error) {
	esCfg := config.GetPrimary()
	w, err := newEsSpanWriter(*esCfg, params.Logger, false, config.Name())
	if err != nil {
		return nil, err
	}
	if config.Primary.IsCreateIndexTemplates() {
		spanMapping, serviceMapping := es.GetSpanServiceMappings(esCfg.GetNumShards(), esCfg.GetNumReplicas(), uint(w.esClientVersion()))
		if err = w.CreateTemplates(ctx, spanMapping, serviceMapping); err != nil {
			return nil, err
		}
	}
	return exporterhelper.NewTraceExporter(
		config,
		params.Logger,
		w.WriteTraces,
		exporterhelper.WithTimeout(config.TimeoutSettings),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithRetry(config.RetrySettings),
		exporterhelper.WithShutdown(func(ctx context.Context) error {
			return esCfg.TLS.Close()
		}))
}
