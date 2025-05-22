// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("jaeger_storage_exporter")

// ID is the identifier of this extension.
var ID = component.NewID(componentType)

// NewFactory creates a factory for jaeger_storage_exporter.
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
	ex := newExporter(cfg, set.TelemetrySettings)
	return exporterhelper.NewTraces(ctx, set, cfg,
		ex.pushTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0}),
		exporterhelper.WithRetry(cfg.RetryConfig),
		exporterhelper.WithQueue(cfg.QueueConfig),
		exporterhelper.WithStart(ex.start),
		exporterhelper.WithShutdown(ex.close),
	)
}
