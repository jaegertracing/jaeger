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

package badgerexporter

import (
	"context"
	"sync"

	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/storage"
)

const (
	// TypeStr defines type of the Badger exporter.
	TypeStr = "jaeger_badger"
)

// OptionsFactory returns initialized badger.Options structure.
type OptionsFactory func() *badger.Options

// DefaultOptions creates Badger options supported by this exporter.
func DefaultOptions() *badger.Options {
	return badger.NewOptions("badger")
}

// Factory is the factory for Jaeger Cassandra exporter.
type Factory struct {
	mutex          *sync.Mutex
	optionsFactory OptionsFactory
}

// NewFactory creates new Factory instance.
func NewFactory(optionsFactory OptionsFactory) *Factory {
	return &Factory{
		optionsFactory: optionsFactory,
		mutex:          &sync.Mutex{},
	}
}

var _ component.ExporterFactory = (*Factory)(nil)

// singleton instance of the factory
// the badger exporter factory always returns this instance
// the singleton instance is shared between OTEL collector and query service
var instance storage.Factory

// GetFactory returns singleton instance of the storage factory.
func GetFactory() storage.Factory {
	return instance
}

// Type gets the type of exporter.
func (f Factory) Type() configmodels.Type {
	return TypeStr
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ExporterFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Exporter {
	opts := f.optionsFactory()
	return &Config{
		ExporterSettings: configmodels.ExporterSettings{
			TypeVal: TypeStr,
			NameVal: TypeStr,
		},
		TimeoutSettings: exporterhelper.DefaultTimeoutSettings(),
		RetrySettings:   exporterhelper.DefaultRetrySettings(),
		QueueSettings:   exporterhelper.DefaultQueueSettings(),
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
	factory, err := f.createStorageFactory(params, config)
	if err != nil {
		return nil, err
	}
	return exporter.NewSpanWriterExporter(cfg, params, factory,
		exporterhelper.WithTimeout(config.TimeoutSettings),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithRetry(config.RetrySettings))
}

// CreateMetricsExporter is not implemented.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateMetricsExporter(
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

func (f Factory) createStorageFactory(params component.ExporterCreateParams, cfg *Config) (storage.Factory, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if instance != nil {
		return instance, nil
	}
	factory := badger.NewFactory()
	factory.InitFromOptions(cfg.Options)
	err := factory.Initialize(metrics.NullFactory, params.Logger)
	if err != nil {
		return nil, err
	}
	instance = factory
	return factory, nil
}
