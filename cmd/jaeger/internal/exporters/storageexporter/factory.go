// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"time"

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
	return &Config{
		RetryConfig: configretry.BackOffConfig{
			Enabled:             true,
			InitialInterval:     5 * time.Second,
			RandomizationFactor: 0.5,
			Multiplier:          1.5,
			MaxInterval:         30 * time.Second,
			MaxElapsedTime:      5 * time.Minute,
		},
	}
}

func createTracesExporter(
	ctx context.Context,
	set exporter.Settings,
	config component.Config,
) (exporter.Traces, error) {
	cfg := config.(*Config)

	defaultCfg := createDefaultConfig().(*Config)

	if !cfg.RetryConfig.Enabled {
		cfg.RetryConfig = configretry.BackOffConfig{
			Enabled: false,
		}
	} else {
		if cfg.RetryConfig.InitialInterval == 0 {
			cfg.RetryConfig.InitialInterval = defaultCfg.RetryConfig.InitialInterval
		}
		if cfg.RetryConfig.MaxInterval == 0 {
			cfg.RetryConfig.MaxInterval = defaultCfg.RetryConfig.MaxInterval
		}
		if cfg.RetryConfig.MaxElapsedTime == 0 {
			cfg.RetryConfig.MaxElapsedTime = defaultCfg.RetryConfig.MaxElapsedTime
		}
		if cfg.RetryConfig.RandomizationFactor == 0 {
			cfg.RetryConfig.RandomizationFactor = defaultCfg.RetryConfig.RandomizationFactor
		}
		if cfg.RetryConfig.Multiplier == 0 {
			cfg.RetryConfig.Multiplier = defaultCfg.RetryConfig.Multiplier
		}
		cfg.RetryConfig.Enabled = true
	}

	ex := newExporter(cfg, set.TelemetrySettings)

	return exporterhelper.NewTraces(
		ctx,
		set,
		cfg,
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
