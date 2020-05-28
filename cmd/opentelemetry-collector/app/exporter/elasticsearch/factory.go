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

package elasticsearch

import (
	"context"
	"fmt"

	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configerror"
	"go.opentelemetry.io/collector/config/configmodels"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry-collector/app/exporter"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/storage"
)

const (
	// TypeStr defines type of the Elasticsearch exporter.
	TypeStr = "jaeger_elasticsearch"
)

// OptionsFactory returns initialized es.OptionsFactory structure.
type OptionsFactory func() *es.Options

// DefaultOptions creates Elasticsearch options supported by this exporter.
func DefaultOptions(enableArchive bool) *es.Options {
	if enableArchive {
		return es.NewOptions("es", "es-archive")
	}
	return es.NewOptions("es")
}

// Factory is the factory for Jaeger Elasticsearch exporter.
type Factory struct {
	OptionsFactory OptionsFactory
}

// Type gets the type of exporter.
func (Factory) Type() configmodels.Type {
	return TypeStr
}

var _ component.ExporterFactory = (*Factory)(nil)
var _ exporter.FactoryCreator = (*Factory)(nil)

// CreateStorageFactory creates Jaeger storage factory.
func (Factory) CreateStorageFactory(params component.ExporterCreateParams, cfg configmodels.Exporter) (storage.Factory, error) {
	esCfg, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	factory := es.NewFactory()
	factory.InitFromOptions(esCfg.Options)
	err := factory.Initialize(metrics.NullFactory, params.Logger)
	if err != nil {
		return nil, err
	}
	return factory, err
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ExporterFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Exporter {
	opts := f.OptionsFactory()
	return &Config{
		Options: *opts,
		ExporterSettings: configmodels.ExporterSettings{
			TypeVal: TypeStr,
			NameVal: TypeStr,
		},
	}
}

// CreateTraceExporter creates Jaeger Elasticsearch trace exporter.
// This function implements OTEL component.ExporterFactory interface.
func (f Factory) CreateTraceExporter(
	_ context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TraceExporter, error) {
	factory, err := f.CreateStorageFactory(params, cfg)
	if err != nil {
		return nil, err
	}
	return exporter.NewSpanWriterExporter(cfg, factory)
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
