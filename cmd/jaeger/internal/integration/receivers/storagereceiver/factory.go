// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagereceiver

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("jaeger_storage_receiver")

// ID is the identifier of this extension.
var ID = component.NewID(componentType)

func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		componentType,
		createDefaultConfig,
		receiver.WithTraces(createTracesReceiver, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

func createTracesReceiver(ctx context.Context, set receiver.CreateSettings, config component.Config, nextConsumer consumer.Traces) (receiver.Traces, error) {
	cfg := config.(*Config)

	return newTracesReceiver(cfg, set, nextConsumer)
}
