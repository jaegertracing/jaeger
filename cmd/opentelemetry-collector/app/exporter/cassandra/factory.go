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
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"

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
	config, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	return New(config, params)
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
