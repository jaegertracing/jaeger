// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"context"
	"fmt"

	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/remotesampling"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/adaptive"
)

type traceProcessor struct {
	config     *Config
	logger     *zap.Logger
	aggregator strategystore.Aggregator
}

func newTraceProcessor(cfg Config, otel component.TelemetrySettings) *traceProcessor {
	return &traceProcessor{
		config: &cfg,
		logger: otel.Logger,
	}
}

func (tp *traceProcessor) start(_ context.Context, host component.Host) error {
	extCfg, err := remotesampling.GetExtensionConfig(host)

	tp.logger.Info("Adaptive sampling extension config", zap.Any("config", extCfg))

	if err != nil {
		return err
	}
	opts := adaptive.Options{
		InitialSamplingProbability:   extCfg.InitialSamplingProbability,
		TargetSamplesPerSecond:       tp.config.TargetSamplesPerSecond,
		DeltaTolerance:               tp.config.DeltaTolerance,
		CalculationInterval:          tp.config.CalculationInterval,
		AggregationBuckets:           extCfg.AggregationBuckets,
		BucketsForCalculation:        tp.config.BucketsForCalculation,
		Delay:                        tp.config.Delay,
		MinSamplingProbability:       tp.config.MinSamplingProbability,
		LeaderLeaseRefreshInterval:   extCfg.LeaderLeaseRefreshInterval,
		FollowerLeaseRefreshInterval: extCfg.FollowerLeaseRefreshInterval,
	}

	store, ep, err := remotesampling.GetSamplingStorage(tp.config.StrategyStore, host, opts, tp.logger)
	if err != nil {
		return err
	}

	agg, err := adaptive.NewAggregator(opts, tp.logger, metrics.NullFactory, ep, store)
	if err != nil {
		return err
	}

	agg.Start()
	tp.aggregator = agg

	return nil
}

func (tp *traceProcessor) close(context.Context) error {
	if tp.aggregator != nil {
		if err := tp.aggregator.Close(); err != nil {
			return fmt.Errorf("failed to stop the adpative sampling aggregator : %w", err)
		}
	}
	return nil
}

func (tp *traceProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	batches, err := otlp2jaeger.ProtoFromTraces(td)
	if err != nil {
		return td, fmt.Errorf("cannot transform OTLP traces to Jaeger format: %w", err)
	}

	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
			adaptive.RecordThroughput(tp.aggregator, span, tp.logger)
		}
	}
	return td, nil
}
