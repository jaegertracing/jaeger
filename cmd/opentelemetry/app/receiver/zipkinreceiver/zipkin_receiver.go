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

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver/zipkinreceiver"

	collectorApp "github.com/jaegertracing/jaeger/cmd/collector/app"
)

// Factory wraps zipkinreceiver.Factory and makes the default config configurable via viper.
// For instance this enables using flags as default values in the config object.
type Factory struct {
	Wrapped component.ReceiverFactory
	// Viper is used to get configuration values for default configuration
	Viper *viper.Viper
}

var _ component.ReceiverFactory = (*Factory)(nil)

// Type returns the type of the receiver.
func (f Factory) Type() configmodels.Type {
	return f.Wrapped.Type()
}

// CreateDefaultConfig returns default configuration of Factory.
// This function implements OTEL component.ReceiverFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Receiver {
	cfg := f.Wrapped.CreateDefaultConfig().(*zipkinreceiver.Config)

	// using the CollectorOptions to parse the zipkin host port b/c it has special processing
	//  for combining the port and host:port zipkin flags
	collectorOpts := &collectorApp.CollectorOptions{}
	collectorOpts.InitFromViper(f.Viper)
	cfg.Endpoint = collectorOpts.CollectorZipkinHTTPHostPort

	return cfg
}

// CreateTracesReceiver creates Zipkin receiver trace receiver.
// This function implements OTEL component.ReceiverFactoryOld interface.
func (f Factory) CreateTracesReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.TracesConsumer,
) (component.TracesReceiver, error) {
	return f.Wrapped.CreateTracesReceiver(ctx, params, cfg, nextConsumer)
}

// CreateMetricsReceiver creates a metrics receiver based on provided config.
// This function implements component.ReceiverFactoryOld.
func (f Factory) CreateMetricsReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	consumer consumer.MetricsConsumer,
) (component.MetricsReceiver, error) {
	return f.Wrapped.CreateMetricsReceiver(ctx, params, cfg, consumer)
}

// CreateLogsReceiver creates a receiver based on the config.
// If the receiver type does not support logs or if the config is not valid
// error will be returned instead.
func (f Factory) CreateLogsReceiver(
	ctx context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.LogsConsumer,
) (component.LogsReceiver, error) {
	return f.Wrapped.CreateLogsReceiver(ctx, params, cfg, nextConsumer)
}
