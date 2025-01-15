// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

type traceProcessor struct {
	config     *Config
	aggregator samplingstrategy.Aggregator
	telset     component.TelemetrySettings
}

func newTraceProcessor(cfg Config, telset component.TelemetrySettings) *traceProcessor {
	return &traceProcessor{
		config: &cfg,
		telset: telset,
	}
}

func (tp *traceProcessor) start(_ context.Context, host component.Host) error {
	parts, err := remotesampling.GetAdaptiveSamplingComponents(host)
	if err != nil {
		return fmt.Errorf(
			"cannot load adaptive sampling components from `%s` extension: %w",
			remotesampling.ComponentType, err)
	}

	agg, err := adaptive.NewAggregator(
		*parts.Options,
		tp.telset.Logger,
		otelmetrics.NewFactory(tp.telset.MeterProvider),
		parts.DistLock,
		parts.SamplingStore,
	)
	if err != nil {
		return fmt.Errorf("failed to create the adaptive sampling aggregator: %w", err)
	}

	agg.Start()
	tp.aggregator = agg

	return nil
}

func (tp *traceProcessor) close(context.Context) error {
	if tp.aggregator != nil {
		if err := tp.aggregator.Close(); err != nil {
			return fmt.Errorf("failed to stop the adaptive sampling aggregator : %w", err)
		}
	}
	return nil
}

func (tp *traceProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	batches := v1adapter.ProtoFromTraces(td)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
			tp.aggregator.HandleRootSpan(span, tp.telset.Logger)
		}
	}
	return td, nil
}
