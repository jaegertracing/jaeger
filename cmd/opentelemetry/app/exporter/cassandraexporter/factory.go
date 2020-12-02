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

package cassandraexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	collector_app "github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
)

const (
	// TypeStr defines type of the Cassandra exporter.
	TypeStr = "jaeger_cassandra"
)

// OptionsFactory returns initialized cassandra.OptionsFactory structure.
type OptionsFactory func() *cassandra.Options

// DefaultOptions creates Cassandra options supported by this exporter.
func DefaultOptions() *cassandra.Options {
	return cassandra.NewOptions("cassandra")
}

// Factory is the factory for Jaeger Cassandra exporter.
type Factory struct {
	OptionsFactory OptionsFactory
}

var _ component.ExporterFactory = (*Factory)(nil)

// Type gets the type of exporter.
func (Factory) Type() configmodels.Type {
	return TypeStr
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ExporterFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Exporter {
	opts := f.OptionsFactory()
	queueSettings := exporterhelper.DefaultQueueSettings()
	queueSettings.NumConsumers = collector_app.DefaultNumWorkers
	queueSettings.QueueSize = collector_app.DefaultQueueSize

	return &Config{
		ExporterSettings: configmodels.ExporterSettings{
			TypeVal: TypeStr,
			NameVal: TypeStr,
		},
		TimeoutSettings: exporterhelper.DefaultTimeoutSettings(),
		RetrySettings:   exporterhelper.DefaultRetrySettings(),
		QueueSettings:   queueSettings,
		Options:         *opts,
	}
}

// CreateTracesExporter creates Jaeger Cassandra trace exporter.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateTracesExporter(
	_ context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TracesExporter, error) {
	config := cfg.(*Config)
	return new(config, params)
}

// CreateMetricsExporter is not implemented.
// This function implements OTEL component.ExporterFactory interface.
func (Factory) CreateMetricsExporter(
	_ context.Context,
	_ component.ExporterCreateParams,
	_ configmodels.Exporter,
) (component.MetricsExporter, error) {
	return nil, configerror.ErrDataTypeIsNotSupported
}

// CreateLogsExporter creates a metrics exporter based on provided config.
// This function implements component.ExporterFactory.
func (f Factory) CreateLogsExporter(
	ctx context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.LogsExporter, error) {
	return nil, configerror.ErrDataTypeIsNotSupported
}
