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

package jaegerreceiver

import (
	"context"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
)

// Factory wraps jaegerreceiver.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	// Wrapped is Jaeger receiver.
	Wrapped *jaegerreceiver.Factory
	// Viper is used to get configuration values for default configuration
	Viper *viper.Viper
}

var _ component.ReceiverFactory = (*Factory)(nil)

// Type gets the type of exporter.
func (f *Factory) Type() configmodels.Type {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.BaseFactory interface.
func (f *Factory) CreateDefaultConfig() configmodels.Receiver {
	cfg := f.Wrapped.CreateDefaultConfig().(*jaegerreceiver.Config)
	strategyFile := f.Viper.GetString(static.SamplingStrategiesFile)
	var samplingConf *jaegerreceiver.RemoteSamplingConfig
	if strategyFile != "" {
		samplingConf = &jaegerreceiver.RemoteSamplingConfig{
			StrategyFile: strategyFile,
		}
	}
	cfg.RemoteSampling = samplingConf
	return cfg
}

// CreateTraceReceiver creates Jaeger receiver trace receiver.
// This function implements OTEL component.ReceiverFactory interface.
func (f *Factory) CreateTraceReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.TraceConsumer,
) (component.TraceReceiver, error) {
	return f.Wrapped.CreateTraceReceiver(ctx, params, cfg, nextConsumer)
}

// CustomUnmarshaler creates custom unmarshaller for Jaeger receiver config.
// This function implements component.ReceiverFactoryBase interface.
func (f *Factory) CustomUnmarshaler() component.CustomUnmarshaler {
	return f.Wrapped.CustomUnmarshaler()
}

// CreateMetricsReceiver creates a metrics receiver based on provided config.
// This function implements component.ReceiverFactory.
func (f *Factory) CreateMetricsReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.MetricsConsumer,
) (component.MetricsReceiver, error) {
	return f.Wrapped.CreateMetricsReceiver(ctx, params, cfg, nextConsumer)
}
