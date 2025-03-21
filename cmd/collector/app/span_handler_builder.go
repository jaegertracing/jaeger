// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zs "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// SpanHandlerBuilder holds configuration required for handlers
type SpanHandlerBuilder struct {
	TraceWriter    tracestore.Writer
	CollectorOpts  *flags.CollectorOptions
	Logger         *zap.Logger
	MetricsFactory api.Factory
	TenancyMgr     *tenancy.Manager
}

// SpanHandlers holds instances to the span handlers built by the SpanHandlerBuilder
type SpanHandlers struct {
	ZipkinSpansHandler   handler.ZipkinSpansHandler
	JaegerBatchesHandler handler.JaegerBatchesHandler
	GRPCHandler          *handler.GRPCHandler
}

// BuildSpanProcessor builds the span processor to be used with the handlers
func (b *SpanHandlerBuilder) BuildSpanProcessor(additional ...ProcessSpan) (processor.SpanProcessor, error) {
	hostname, _ := os.Hostname()
	svcMetrics := b.metricsFactory()
	hostMetrics := svcMetrics.Namespace(api.NSOptions{Tags: map[string]string{"host": hostname}})

	return NewSpanProcessor(
		b.TraceWriter,
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

func (b *SpanHandlerBuilder) metricsFactory() api.Factory {
	if b.MetricsFactory == nil {
		return api.NullFactory
	}
	return b.MetricsFactory
}
