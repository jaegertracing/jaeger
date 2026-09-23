// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// componentType is the name of this component in configuration. The same type
// is registered as an exporter (NewFactory) and as a connector
// (NewConnectorFactory): the collector keeps the two kinds in separate factory
// maps and resolves a pipeline entry as a connector only when it is declared
// under `connectors:`, so the section an operator puts jaeger_storage_exporter in
// selects the plain storage write or the write with a dead-letter output for the
// spans the storage rejects (RFC 0007 §4.8).
var componentType = component.MustNewType("jaeger_storage_exporter")

// ID is the identifier of this component.
var ID = component.NewID(componentType)

// NewFactory creates the exporter factory for jaeger_storage_exporter.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		componentType,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	cfg := configretry.NewDefaultBackOffConfig()
	cfg.Enabled = false
	return &Config{
		RetryConfig: cfg,
	}
}

func createTracesExporter(ctx context.Context, set exporter.Settings, config component.Config) (exporter.Traces, error) {
	cfg := config.(*Config)
	ex := NewTraceWriter(cfg, set.TelemetrySettings)
	return exporterhelper.NewTraces(
		ctx, set, cfg,
		ex.WriteTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0}),
		exporterhelper.WithRetry(cfg.RetryConfig),
		exporterhelper.WithQueue(cfg.QueueConfig),
		exporterhelper.WithStart(ex.Start),
		exporterhelper.WithShutdown(ex.close),
	)
}

// NewConnectorFactory creates the connector factory for jaeger_storage_exporter:
// the same storage write with an output pipeline for the spans the storage
// rejected terminally (RFC 0007 §4.8). It shares Config and defaults with the
// exporter, so the two forms behave the same when configured the same.
func NewConnectorFactory() connector.Factory {
	return connector.NewFactory(
		componentType,
		createDefaultConfig,
		connector.WithTracesToTraces(createTracesToTraces, component.StabilityLevelDevelopment),
	)
}

func createTracesToTraces(ctx context.Context, set connector.Settings, config component.Config, next consumer.Traces) (connector.Traces, error) {
	cfg := config.(*Config)
	if cfg.QueueConfig.HasValue() && !cfg.QueueConfig.Get().WaitForResult {
		return nil, errQueueWithoutResult
	}
	return newConnector(ctx, set, cfg, next)
}
