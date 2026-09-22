// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
)

// componentType is the name of this connector in configuration.
var componentType = component.MustNewType("jaeger_storage_writer")

// ID is the identifier of this connector.
var ID = component.NewID(componentType)

// NewFactory creates a factory for the jaeger_storage_writer connector.
func NewFactory() connector.Factory {
	return connector.NewFactory(
		componentType,
		createDefaultConfig,
		connector.WithTracesToTraces(createTracesToTraces, component.StabilityLevelDevelopment),
	)
}

// createDefaultConfig returns jaeger_storage_exporter's defaults, so the two
// components behave the same when configured the same.
func createDefaultConfig() component.Config {
	return storageexporter.NewFactory().CreateDefaultConfig()
}

func createTracesToTraces(ctx context.Context, set connector.Settings, cfg component.Config, next consumer.Traces) (connector.Traces, error) {
	return newConnector(ctx, set, cfg.(*Config), next)
}
