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

// componentType is the name of this connector in configuration. It is the same
// type as the jaeger_storage_exporter exporter on purpose: the collector keeps
// exporter and connector factories in separate maps and resolves a pipeline entry
// as a connector only when the operator declared it under `connectors:`, so the
// section an operator puts `jaeger_storage_exporter` in selects the plain
// storage write or the write with a dead-letter output (RFC 0007 §4.8).
var componentType = component.MustNewType("jaeger_storage_exporter")

// ID is the identifier of this connector.
var ID = component.NewID(componentType)

// NewFactory creates the connector factory for jaeger_storage_exporter.
func NewFactory() connector.Factory {
	return connector.NewFactory(
		componentType,
		createDefaultConfig,
		connector.WithTracesToTraces(createTracesToTraces, component.StabilityLevelDevelopment),
	)
}

// createDefaultConfig wraps jaeger_storage_exporter's defaults, so the two
// components behave the same when configured the same.
func createDefaultConfig() component.Config {
	return &Config{Config: *storageexporter.NewFactory().CreateDefaultConfig().(*storageexporter.Config)}
}

func createTracesToTraces(ctx context.Context, set connector.Settings, cfg component.Config, next consumer.Traces) (connector.Traces, error) {
	return newConnector(ctx, set, cfg.(*Config), next)
}
