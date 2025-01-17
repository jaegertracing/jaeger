// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
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
}

func newDependencyProcessor(cfg Config, telset component.TelemetrySettings, dependencyWriter *memory.Store) *dependencyProcessor {
	return &dependencyProcessor{
		config:           &cfg,
		telset:           telset,
		dependencyWriter: dependencyWriter,
		closeChan:        make(chan struct{}),
	}
}

func (tp *dependencyProcessor) start(_ context.Context, host component.Host) error {
	tp.aggregator = newDependencyAggregator(*tp.config, tp.telset, tp.dependencyWriter)
	tp.aggregator.Start(tp.closeChan)
	return nil
}

func (tp *dependencyProcessor) close(ctx context.Context) error {
	close(tp.closeChan)
	if tp.aggregator != nil {
		if err := tp.aggregator.Close(); err != nil {
			return fmt.Errorf("failed to stop the dependency aggregator : %w", err)
		}
	}
	return nil
}

func (tp *dependencyProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	batches := v1adapter.ProtoFromTraces(td)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
			tp.aggregator.HandleSpan(span)
		}
	}
	return td, nil
}
