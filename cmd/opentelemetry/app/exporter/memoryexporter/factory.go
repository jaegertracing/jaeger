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

package memoryexporter

import (
	"context"
	"fmt"
	"sync"

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configmodels"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
)

// TypeStr defines exporter type.
const TypeStr = "jaeger_memory"

// Factory is the factory for Jaeger in-memory exporter.
type Factory struct {
	viper *viper.Viper
	mutex *sync.Mutex
}

// NewFactory creates Factory.
func NewFactory(v *viper.Viper) *Factory {
	return &Factory{
		viper: v,
		mutex: &sync.Mutex{},
	}
}

var _ component.ExporterFactory = (*Factory)(nil)

// singleton instance of the factory
// the in-memory exporter factory always returns this instance
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
	opts := memory.Options{}
	opts.InitFromViper(f.viper)
	return &Config{
		Options: opts,
		ExporterSettings: configmodels.ExporterSettings{
			TypeVal: TypeStr,
			NameVal: TypeStr,
		},
	}
}

// CreateTracesExporter creates Jaeger Kafka trace exporter.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateTracesExporter(
	_ context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TracesExporter, error) {
	factory, err := f.createStorageFactory(params, cfg)
	if err != nil {
		return nil, err
	}
	return exporter.NewSpanWriterExporter(cfg, params, factory)
}

// CreateMetricsExporter is not implemented.
// This function implements OTEL component.Factory interface.
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

func (f Factory) createStorageFactory(params component.ExporterCreateParams, cfg configmodels.Exporter) (storage.Factory, error) {
	config, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if instance != nil {
		return instance, nil
	}
	factory := memory.NewFactory()
	factory.InitFromOptions(config.Options)
	err := factory.Initialize(metrics.NullFactory, params.Logger)
	if err != nil {
		return nil, err
	}
	instance = factory
	return factory, nil
}
