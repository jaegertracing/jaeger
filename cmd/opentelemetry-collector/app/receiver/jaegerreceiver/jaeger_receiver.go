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
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer"
	"github.com/open-telemetry/opentelemetry-collector/receiver/jaegerreceiver"
	"github.com/spf13/viper"
	"go.uber.org/zap"

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

var _ component.ReceiverFactoryOld = (*Factory)(nil)

// Type gets the type of exporter.
func (f *Factory) Type() string {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.BaseFactory interface.
func (f *Factory) CreateDefaultConfig() configmodels.Receiver {
	cfg := f.Wrapped.CreateDefaultConfig().(*jaegerreceiver.Config)
	strategyFile := f.Viper.GetString(static.SamplingStrategiesFile)
	// if remote sampling struct is not nil the factory will use default values for endpoints and enable remote sampling
	// the problem is that flag will always return some default value
	// so we cannot distinguish when it should be enabled and when not.
	// Using default values makes sense when a component is enabled
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
	log *zap.Logger,
	cfg configmodels.Receiver,
	nextConsumer consumer.TraceConsumerOld,
) (component.TraceReceiver, error) {
	return f.Wrapped.CreateTraceReceiver(ctx, log, cfg, nextConsumer)
}

// CustomUnmarshaler creates custom unmarshaller for Jaeger receiver config.
// This function implements component.ReceiverFactoryBase interface.
func (f *Factory) CustomUnmarshaler() component.CustomUnmarshaler {
	return f.Wrapped.CustomUnmarshaler()
}

// CreateMetricsReceiver creates a metrics receiver based on provided config.
// This function implements component.ReceiverFactory.
func (f *Factory) CreateMetricsReceiver(
	_ *zap.Logger,
	_ configmodels.Receiver,
	_ consumer.MetricsConsumerOld,
) (component.MetricsReceiver, error) {
	return nil, configerror.ErrDataTypeIsNotSupported
}
