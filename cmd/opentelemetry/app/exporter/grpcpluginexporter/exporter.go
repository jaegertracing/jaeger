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

package grpcpluginexporter

import (
	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	storageOtelExporter "github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter"
	storageGrpc "github.com/jaegertracing/jaeger/plugin/storage/grpc"
)

// new creates gRPC exporter/storage.
func new(config *Config, params component.ExporterCreateParams) (component.TracesExporter, error) {
	factory := storageGrpc.NewFactory()
	factory.InitFromOptions(config.Options)
	err := factory.Initialize(metrics.NullFactory, params.Logger)
	if err != nil {
		return nil, err
	}
	return storageOtelExporter.NewSpanWriterExporter(&config.ExporterSettings, params, factory,
		exporterhelper.WithTimeout(config.TimeoutSettings),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithRetry(config.RetrySettings))
}
