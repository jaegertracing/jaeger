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

package kafka

import (
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

const (
	TypeStr = "jaeger_kafka"
)

// OptionsFactory returns initialized kafka.Options structure.
type OptionsFactory func() *kafka.Options

// DefaultOptions creates Kafka options supported by this exporter.
func DefaultOptions() *kafka.Options {
	return &kafka.Options{}
}

// Factory is the factory for Jaeger Kafka exporter.
type Factory struct {
	OptionsFactory OptionsFactory
}

// Type gets the type of exporter.
func (Factory) Type() string {
	return TypeStr
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL exporter.BaseFactory interface.
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

// CreateTraceExporter creates Jaeger Kafka trace exporter.
// This function implements OTEL component.Factory interface.
func (Factory) CreateTraceExporter(log *zap.Logger, cfg configmodels.Exporter) (component.TraceExporterOld, error) {
	kafkaCfg, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	return New(kafkaCfg, log)
}

// CreateMetricsExporter is not implemented.
// This function implements OTEL component.Factory interface.
func (Factory) CreateMetricsExporter(*zap.Logger, configmodels.Exporter) (component.MetricsExporterOld, error) {
	return nil, configerror.ErrDataTypeIsNotSupported
}
