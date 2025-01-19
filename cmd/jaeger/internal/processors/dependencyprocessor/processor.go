// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

type dependencyProcessor struct {
	config           *Config
	aggregator       *dependencyAggregator // Define the aggregator below.
	telset           component.TelemetrySettings
	dependencyWriter *memory.Store
	closeChan        chan struct{}
	nextConsumer     consumer.Traces
}

func newDependencyProcessor(cfg Config, telset component.TelemetrySettings, dependencyWriter *memory.Store, nextConsumer consumer.Traces) *dependencyProcessor {
	return &dependencyProcessor{
		config:           &cfg,
		telset:           telset,
		dependencyWriter: dependencyWriter,
		closeChan:        make(chan struct{}),
		nextConsumer:     nextConsumer,
	}
}

func (tp *dependencyProcessor) Start(_ context.Context, host component.Host) error {
	tp.aggregator = newDependencyAggregator(*tp.config, tp.telset, tp.dependencyWriter)
	tp.aggregator.Start(tp.closeChan)
	return nil
}

// Shutdown implements processor.Traces
func (tp *dependencyProcessor) Shutdown(ctx context.Context) error {
	close(tp.closeChan)
	if tp.aggregator != nil {
		if err := tp.aggregator.Close(); err != nil {
			return fmt.Errorf("failed to stop the dependency aggregator : %w", err)
		}
	}
	return nil
}

// Capabilities implements processor.Traces
func (p *dependencyProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (dp *dependencyProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	batches := v1adapter.ProtoFromTraces(td)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
			dp.aggregator.HandleSpan(span)
		}
	}
	return dp.nextConsumer.ConsumeTraces(ctx, td)
}
