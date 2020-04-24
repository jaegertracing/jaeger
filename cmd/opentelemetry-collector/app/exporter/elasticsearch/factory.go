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

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"

	"github.com/jaegertracing/jaeger/plugin/storage/es"
)

const (
	// TypeStr defines type of the Elasticsearch exporter.
	TypeStr = "jaeger_elasticsearch"
)

// OptionsFactory returns initialized es.OptionsFactory structure.
type OptionsFactory func() *es.Options

// DefaultOptions creates Elasticsearch options supported by this exporter.
func DefaultOptions() *es.Options {
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
// This function implements OTEL exporter.ExporterFactory interface.
func (Factory) CreateTraceExporter(
	_ context.Context,
	params component.ExporterCreateParams,
	cfg configmodels.Exporter,
) (component.TraceExporter, error) {
	esCfg, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	return New(esCfg, params)
}

// CreateMetricsExporter is not implemented.
// This function implements OTEL exporter.ExporterFactory interface.
func (Factory) CreateMetricsExporter(
	_ context.Context,
	_ component.ExporterCreateParams,
	_ configmodels.Exporter,
) (component.MetricsExporter, error) {
	return nil, configerror.ErrDataTypeIsNotSupported
}
