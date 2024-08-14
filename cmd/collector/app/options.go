// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type options struct {
	logger                 *zap.Logger
	serviceMetrics         metrics.Factory
	hostMetrics            metrics.Factory
	preProcessSpans        ProcessSpans // see docs in PreProcessSpans option.
	sanitizer              sanitizer.SanitizeSpan
	preSave                ProcessSpan
	spanFilter             FilterSpan
	numWorkers             int
	blockingSubmit         bool
	queueSize              int
	dynQueueSizeWarmup     uint
	dynQueueSizeMemory     uint
	reportBusy             bool
	extraFormatTypes       []processor.SpanFormat
	collectorTags          map[string]string
	spanSizeMetricsEnabled bool
	onDroppedSpan          func(span *model.Span)
}

// Option is a function that sets some option on StorageBuilder.
type Option func(c *options)

// Options is a factory for all available Option's
var Options options

// Logger creates a Option that initializes the logger
func (options) Logger(logger *zap.Logger) Option {
	return func(b *options) {
		b.logger = logger
	}
}

// ServiceMetrics creates an Option that initializes the serviceMetrics metrics factory
func (options) ServiceMetrics(serviceMetrics metrics.Factory) Option {
	return func(b *options) {
		b.serviceMetrics = serviceMetrics
	}
}

// HostMetrics creates an Option that initializes the hostMetrics metrics factory
func (options) HostMetrics(hostMetrics metrics.Factory) Option {
	return func(b *options) {
		b.hostMetrics = hostMetrics
	}
}

// PreProcessSpans creates an Option that initializes the preProcessSpans function.
// This function can implement non-standard pre-processing of the spans when extending
// the collector from source. Jaeger itself does not define any pre-processing.
func (options) PreProcessSpans(preProcessSpans ProcessSpans) Option {
	return func(b *options) {
		b.preProcessSpans = preProcessSpans
	}
}

// Sanitizer creates an Option that initializes the sanitizer function
func (options) Sanitizer(sanitizer sanitizer.SanitizeSpan) Option {
	return func(b *options) {
		b.sanitizer = sanitizer
	}
}

// PreSave creates an Option that initializes the preSave function
func (options) PreSave(preSave ProcessSpan) Option {
	return func(b *options) {
		b.preSave = preSave
	}
}

// SpanFilter creates an Option that initializes the spanFilter function
func (options) SpanFilter(spanFilter FilterSpan) Option {
	return func(b *options) {
		b.spanFilter = spanFilter
	}
}

// NumWorkers creates an Option that initializes the number of queue consumers AKA workers
func (options) NumWorkers(numWorkers int) Option {
	return func(b *options) {
		b.numWorkers = numWorkers
	}
}

// BlockingSubmit creates an Option that initializes the blockingSubmit boolean
func (options) BlockingSubmit(blockingSubmit bool) Option {
	return func(b *options) {
		b.blockingSubmit = blockingSubmit
	}
}

// QueueSize creates an Option that initializes the queue size
func (options) QueueSize(queueSize int) Option {
	return func(b *options) {
		b.queueSize = queueSize
	}
}

// DynQueueSizeWarmup creates an Option that initializes the dynamic queue size
func (options) DynQueueSizeWarmup(dynQueueSizeWarmup uint) Option {
	return func(b *options) {
		b.dynQueueSizeWarmup = dynQueueSizeWarmup
	}
}

// DynQueueSizeMemory creates an Option that initializes the dynamic queue memory
func (options) DynQueueSizeMemory(dynQueueSizeMemory uint) Option {
	return func(b *options) {
		b.dynQueueSizeMemory = dynQueueSizeMemory
	}
}

// ReportBusy creates an Option that initializes the reportBusy boolean
func (options) ReportBusy(reportBusy bool) Option {
	return func(b *options) {
		b.reportBusy = reportBusy
	}
}

// ExtraFormatTypes creates an Option that initializes the extra list of format types
func (options) ExtraFormatTypes(extraFormatTypes []processor.SpanFormat) Option {
	return func(b *options) {
		b.extraFormatTypes = extraFormatTypes
	}
}

// CollectorTags creates an Option that initializes the extra tags to append to the spans flowing through this collector
func (options) CollectorTags(extraTags map[string]string) Option {
	return func(b *options) {
		b.collectorTags = extraTags
	}
}

// SpanSizeMetricsEnabled creates an Option that initializes the spanSizeMetrics boolean
func (options) SpanSizeMetricsEnabled(spanSizeMetrics bool) Option {
	return func(b *options) {
		b.spanSizeMetricsEnabled = spanSizeMetrics
	}
}

// OnDroppedSpan creates an Option that initializes the onDroppedSpan function
func (options) OnDroppedSpan(onDroppedSpan func(span *model.Span)) Option {
	return func(b *options) {
		b.onDroppedSpan = onDroppedSpan
	}
}

func (options) apply(opts ...Option) options {
	ret := options{}
	for _, opt := range opts {
		opt(&ret)
	}
	if ret.logger == nil {
		ret.logger = zap.NewNop()
	}
	if ret.serviceMetrics == nil {
		ret.serviceMetrics = metrics.NullFactory
	}
	if ret.hostMetrics == nil {
		ret.hostMetrics = metrics.NullFactory
	}
	if ret.preProcessSpans == nil {
		ret.preProcessSpans = func(_ []*model.Span, _ /* tenant */ string) {}
	}
	if ret.sanitizer == nil {
		ret.sanitizer = func(span *model.Span) *model.Span { return span }
	}
	if ret.preSave == nil {
		ret.preSave = func(_ *model.Span, _ /* tenant */ string) {}
	}
	if ret.spanFilter == nil {
		ret.spanFilter = func(_ *model.Span) bool { return true }
	}
	if ret.numWorkers == 0 {
		ret.numWorkers = flags.DefaultNumWorkers
	}
	return ret
}
