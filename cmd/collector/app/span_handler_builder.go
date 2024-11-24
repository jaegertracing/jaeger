// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zs "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// SpanHandlerBuilder holds configuration required for handlers
type SpanHandlerBuilder struct {
	SpanWriter     spanstore.Writer
	CollectorOpts  *flags.CollectorOptions
	Logger         *zap.Logger
	MetricsFactory metrics.Factory
	TenancyMgr     *tenancy.Manager
}

// SpanHandlers holds instances to the span handlers built by the SpanHandlerBuilder
type SpanHandlers struct {
	ZipkinSpansHandler   handler.ZipkinSpansHandler
	JaegerBatchesHandler handler.JaegerBatchesHandler
	GRPCHandler          *handler.GRPCHandler
}

// BuildSpanProcessor builds the span processor to be used with the handlers
func (b *SpanHandlerBuilder) BuildSpanProcessor(additional ...ProcessSpan) processor.SpanProcessor {
	hostname, _ := os.Hostname()
	svcMetrics := b.metricsFactory()
	hostMetrics := svcMetrics.Namespace(metrics.NSOptions{Tags: map[string]string{"host": hostname}})

	return NewSpanProcessor(
		b.SpanWriter,
		additional,
		Options.ServiceMetrics(svcMetrics),
		Options.HostMetrics(hostMetrics),
		Options.Logger(b.logger()),
		Options.SpanFilter(defaultSpanFilter),
		Options.NumWorkers(b.CollectorOpts.NumWorkers),
		//nolint: gosec // G115
		Options.QueueSize(int(b.CollectorOpts.QueueSize)),
		Options.CollectorTags(b.CollectorOpts.CollectorTags),
		Options.DynQueueSizeWarmup(b.CollectorOpts.QueueSize), // same as queue size for now
		Options.DynQueueSizeMemory(b.CollectorOpts.DynQueueSizeMemory),
		Options.SpanSizeMetricsEnabled(b.CollectorOpts.SpanSizeMetricsEnabled),
	)
}

// BuildHandlers builds span handlers (Zipkin, Jaeger)
func (b *SpanHandlerBuilder) BuildHandlers(spanProcessor processor.SpanProcessor) *SpanHandlers {
	return &SpanHandlers{
		handler.NewZipkinSpanHandler(
			b.Logger,
			spanProcessor,
			zs.NewChainedSanitizer(zs.NewStandardSanitizers()...),
		),
		handler.NewJaegerSpanHandler(b.Logger, spanProcessor),
		handler.NewGRPCHandler(b.Logger, spanProcessor, b.TenancyMgr),
	}
}

func defaultSpanFilter(*model.Span) bool {
	return true
}

func (b *SpanHandlerBuilder) logger() *zap.Logger {
	if b.Logger == nil {
		return zap.NewNop()
	}
	return b.Logger
}

func (b *SpanHandlerBuilder) metricsFactory() metrics.Factory {
	if b.MetricsFactory == nil {
		return metrics.NullFactory
	}
	return b.MetricsFactory
}
