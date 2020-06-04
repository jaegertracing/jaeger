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
	"fmt"
	"sync"

	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configmodels"

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
		Options: *opts,
		ExporterSettings: configmodels.ExporterSettings{
			TypeVal: TypeStr,
			NameVal: TypeStr,
		},
	}
}

// CreateTraceExporter creates Jaeger Cassandra trace exporter.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateTraceExporter(
	_ context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TraceExporter, error) {
	factory, err := f.createStorageFactory(params, cfg)
	if err != nil {
		return nil, err
	}
	return exporter.NewSpanWriterExporter(cfg, factory)
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

func (f Factory) createStorageFactory(params component.ExporterCreateParams, cfg configmodels.Exporter) (storage.Factory, error) {
	config, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	f.mutex.Lock()
	if instance != nil {
		return instance, nil
	}
	factory := badger.NewFactory()
	factory.InitFromOptions(config.Options)
	err := factory.Initialize(metrics.NullFactory, params.Logger)
	if err != nil {
		return nil, err
	}
	instance = factory
	f.mutex.Unlock()
	return factory, nil
}
