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

package zipkinreceiver

import (
	"context"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer"
	"github.com/open-telemetry/opentelemetry-collector/receiver/zipkinreceiver"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
)

// Factory wraps zipkinreceiver.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	Wrapped *zipkinreceiver.Factory
	// Viper is used to get configuration values for default configuration
	Viper *viper.Viper
}

var _ component.ReceiverFactoryOld = (*Factory)(nil)

// Type returns the type of the receiver.
func (f Factory) Type() configmodels.Type {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ReceiverFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Receiver {
	cfg := f.Wrapped.CreateDefaultConfig().(*zipkinreceiver.Config)
	if f.Viper.IsSet(collectorApp.CollectorZipkinHTTPHostPort) {
		cfg.Endpoint = f.Viper.GetString(collectorApp.CollectorZipkinHTTPHostPort)
	}
	return cfg
}

// CustomUnmarshaler creates custom unmarshaller for Zipkin receiver config.
// This function implements component.ReceiverFactoryBase interface.
func (f Factory) CustomUnmarshaler() component.CustomUnmarshaler {
	return f.Wrapped.CustomUnmarshaler()
}

// CreateTraceReceiver creates Zipkin receiver trace receiver.
// This function implements OTEL component.ReceiverFactoryOld interface.
func (f Factory) CreateTraceReceiver(
	ctx context.Context,
	logger *zap.Logger,
	cfg configmodels.Receiver,
	nextConsumer consumer.TraceConsumerOld,
) (component.TraceReceiver, error) {
	return f.Wrapped.CreateTraceReceiver(ctx, logger, cfg, nextConsumer)
}

// CreateMetricsReceiver creates a metrics receiver based on provided config.
// This function implements component.ReceiverFactoryOld.
func (f Factory) CreateMetricsReceiver(
	logger *zap.Logger,
	cfg configmodels.Receiver,
	consumer consumer.MetricsConsumerOld,
) (component.MetricsReceiver, error) {
	return f.Wrapped.CreateMetricsReceiver(logger, cfg, consumer)
}
