// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"

	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

// componentType is the name of this processor in configuration.
var componentType = component.MustNewType("dependencyprocessor")

// NewFactory creates a factory for the dependency processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		componentType,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelAlpha),
	)
}

// createDefaultConfig returns the default configuration for the dependency processor.
func createDefaultConfig() component.Config {
	return &Config{
		AggregationInterval: 5 * time.Second,
		InactivityTimeout:   2 * time.Second,
		Store:               memory.NewStore(),
	}
}

// createTracesProcessor creates a new instance of the dependency processor.
func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	oCfg := cfg.(*Config)

	dp := newDependencyProcessor(*oCfg, set.TelemetrySettings, oCfg.Store, nextConsumer)

	return dp, nil
}
