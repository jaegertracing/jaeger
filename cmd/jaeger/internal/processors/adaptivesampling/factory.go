// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("adaptive_sampling")

const (
	defaultTargetSamplesPerSecond = 1
	defaultDeltaTolerance         = 0.3
	defaultBucketsForCalculation  = 1
	defaultCalculationInterval    = time.Minute
	defaultAggregationBuckets     = 10
	defaultDelay                  = time.Minute * 2
	defaultMinSamplingProbability = 1e-5 // one in 100k requests
)

// NewFactory creates a factory for the jaeger remote sampling extension.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		componentType,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelBeta),
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.CreateSettings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	oCfg := cfg.(*Config)
	sp := newTraceProcessor(*oCfg, set.TelemetrySettings)
	return processorhelper.NewTracesProcessor(
		ctx,
		set,
		cfg,
		nextConsumer,
		sp.processTraces,
		processorhelper.WithStart(sp.start),
		processorhelper.WithShutdown(sp.close),
	)
}
