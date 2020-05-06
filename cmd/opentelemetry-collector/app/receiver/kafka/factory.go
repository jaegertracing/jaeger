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
	"context"
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/config/configerror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer"

	ingesterApp "github.com/jaegertracing/jaeger/cmd/ingester/app"
)

const (
	TypeStr = "jaeger_kafka"
)

// OptionsFactory returns initialized ingester app.Options structure.
type OptionsFactory func() *ingesterApp.Options

// DefaultOptions creates Kafka options supported by this receiver.
func DefaultOptions() *ingesterApp.Options {
	return &ingesterApp.Options{}
}

type Factory struct {
	OptionsFactory OptionsFactory
}

var _ component.ReceiverFactory = (*Factory)(nil)

// Type returns the receiver type.
func (f Factory) Type() configmodels.Type {
	return TypeStr
}

// CreateDefaultConfig creates default config.
// This function implements OTEL component.ReceiverFactoryBase interface.
func (f Factory) CreateDefaultConfig() configmodels.Receiver {
	opts := f.OptionsFactory()
	return &Config{
		Options: *opts,
	}
}

// CustomUnmarshaler returns custom marshaller.
// This function implements OTEL component.ReceiverFactoryBase interface.
func (f Factory) CustomUnmarshaler() component.CustomUnmarshaler {
	return nil
}

// CreateTraceReceiver returns Kafka receiver.
// This function implements OTEL component.ReceiverFactory.
func (f Factory) CreateTraceReceiver(
	_ context.Context,
	params component.ReceiverCreateParams,
	cfg configmodels.Receiver,
	nextConsumer consumer.TraceConsumer,
) (component.TraceReceiver, error) {
	kafkaCfg, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("could not cast configuration to %s", TypeStr)
	}
	return new(kafkaCfg, nextConsumer, params)
}

// CreateMetricsReceiver returns metrics receiver.
// This function implements OTEL component.ReceiverFactory.
func (f Factory) CreateMetricsReceiver(
	_ context.Context,
	_ component.ReceiverCreateParams,
	_ configmodels.Receiver,
	_ consumer.MetricsConsumer,
) (component.MetricsReceiver, error) {
	return nil, configerror.ErrDataTypeIsNotSupported
}
