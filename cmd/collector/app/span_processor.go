// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/queue"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	// if this proves to be too low, we can increase it
	maxQueueSize = 1_000_000

	// if the new queue size isn't 20% bigger than the previous one, don't change
	minRequiredChange = 1.2
)

type spanProcessor struct {
	queue              *queue.BoundedQueue[queueItem]
	queueResizeMu      sync.Mutex
	metrics            *SpanProcessorMetrics
	preProcessSpans    ProcessSpans
	filterSpan         FilterSpan             // filter is called before the sanitizer but after preProcessSpans
	sanitizer          sanitizer.SanitizeSpan // sanitizer is called before processSpan
	processSpan        ProcessSpan
	logger             *zap.Logger
	spanWriter         spanstore.Writer
	reportBusy         bool
	numWorkers         int
	collectorTags      map[string]string
	dynQueueSizeWarmup uint
	dynQueueSizeMemory uint
	bytesProcessed     atomic.Uint64
	spansProcessed     atomic.Uint64
	stopCh             chan struct{}
}

type queueItem struct {
	queuedTime time.Time
	span       *model.Span
	tenant     string
}

// NewSpanProcessor returns a SpanProcessor that preProcesses, filters, queues, sanitizes, and processes spans.
func NewSpanProcessor(
	spanWriter spanstore.Writer,
	additional []ProcessSpan,
	opts ...Option,
) (processor.SpanProcessor, error) {
	sp, err := newSpanProcessor(spanWriter, additional, opts...)
	if err != nil {
		return nil, err
	}

	sp.queue.StartConsumers(sp.numWorkers, func(item queueItem) {
		sp.processItemFromQueue(item)
	})

	sp.background(1*time.Second, sp.updateGauges)

	if sp.dynQueueSizeMemory > 0 {
		sp.background(1*time.Minute, sp.updateQueueSize)
	}

	return sp, nil
}

func newSpanProcessor(spanWriter spanstore.Writer, additional []ProcessSpan, opts ...Option) (*spanProcessor, error) {
	options := Options.apply(opts...)
	handlerMetrics := NewSpanProcessorMetrics(
		options.serviceMetrics,
		options.hostMetrics,
		options.extraFormatTypes)
	droppedItemHandler := func(item queueItem) {
		handlerMetrics.SpansDropped.Inc(1)
		if options.onDroppedSpan != nil {
			options.onDroppedSpan(item.span)
		}
	}
	boundedQueue := queue.NewBoundedQueue[queueItem](options.queueSize, droppedItemHandler)

	sanitizers := sanitizer.NewStandardSanitizers()
	if options.sanitizer != nil {
		sanitizers = append(sanitizers, options.sanitizer)
	}

	sp := spanProcessor{
		queue:              boundedQueue,
		metrics:            handlerMetrics,
		logger:             options.logger,
		preProcessSpans:    options.preProcessSpans,
		filterSpan:         options.spanFilter,
		sanitizer:          sanitizer.NewChainedSanitizer(sanitizers...),
		reportBusy:         options.reportBusy,
		numWorkers:         options.numWorkers,
		spanWriter:         spanWriter,
		collectorTags:      options.collectorTags,
		stopCh:             make(chan struct{}),
		dynQueueSizeMemory: options.dynQueueSizeMemory,
		dynQueueSizeWarmup: options.dynQueueSizeWarmup,
	}

	processSpanFuncs := []ProcessSpan{options.preSave, sp.saveSpan}
	if options.dynQueueSizeMemory > 0 {
		options.logger.Info("Dynamically adjusting the queue size at runtime.",
			zap.Uint("memory-mib", options.dynQueueSizeMemory/1024/1024),
			zap.Uint("queue-size-warmup", options.dynQueueSizeWarmup))
	}
	if options.dynQueueSizeMemory > 0 || options.spanSizeMetricsEnabled {
		// add to processSpanFuncs
		processSpanFuncs = append(processSpanFuncs, sp.countSpan)
	}

	processSpanFuncs = append(processSpanFuncs, additional...)

	sp.processSpan = ChainedProcessSpan(processSpanFuncs...)
	return &sp, nil
}

func (sp *spanProcessor) Close() error {
	close(sp.stopCh)
	sp.queue.Stop()

	return nil
}

func (sp *spanProcessor) saveSpan(span *model.Span, tenant string) {
	if nil == span.Process {
		sp.logger.Error("process is empty for the span")
		sp.metrics.SavedErrBySvc.ReportServiceNameForSpan(span)
		return
	}

	startTime := time.Now()
	// Since we save spans asynchronously from receiving them, we cannot reuse
	// the inbound Context, as it may be cancelled by the time we reach this point,
	// so we need to start a new Context.
	ctx := tenancy.WithTenant(context.Background(), tenant)
	if err := sp.spanWriter.WriteSpan(ctx, span); err != nil {
		sp.logger.Error("Failed to save span", zap.Error(err))
		sp.metrics.SavedErrBySvc.ReportServiceNameForSpan(span)
	} else {
		sp.logger.Debug("Span written to the storage by the collector",
			zap.Stringer("trace-id", span.TraceID), zap.Stringer("span-id", span.SpanID))
		sp.metrics.SavedOkBySvc.ReportServiceNameForSpan(span)
	}
	sp.metrics.SaveLatency.Record(time.Since(startTime))
}

func (sp *spanProcessor) countSpan(span *model.Span, _ string /* tenant */) {
	//nolint: gosec // G115
	sp.bytesProcessed.Add(uint64(span.Size()))
	sp.spansProcessed.Add(1)
}

func (sp *spanProcessor) ProcessSpans(batch processor.Batch) ([]bool, error) {
	sp.preProcessSpans(batch)
	var spans []*model.Span
	batch.GetSpans(func(spansV1 []*model.Span) {
		spans = spansV1
	}, func(_ ptrace.Traces) {
		panic("not implemented")
	})
	sp.metrics.BatchSize.Update(int64(len(spans)))
	retMe := make([]bool, len(spans))

	// Note: this is not the ideal place to do this because collector tags are added to Process.Tags,
	// and Process can be shared between different spans in the batch, but we no longer know that,
	// the relation is lost upstream and it's impossible in Go to dedupe pointers. But at least here
	// we have a single thread updating all spans that may share the same Process, before concurrency
	// kicks in.
	for _, span := range spans {
		sp.addCollectorTags(span)
	}

	for i, mSpan := range spans {
		// TODO does this have to be one span at a time?
		ok := sp.enqueueSpan(mSpan, batch.GetSpanFormat(), batch.GetInboundTransport(), batch.GetTenant())
		if !ok && sp.reportBusy {
			return nil, processor.ErrBusy
		}
		retMe[i] = ok
	}
	return retMe, nil
}

func (sp *spanProcessor) processItemFromQueue(item queueItem) {
	// TODO calling sanitizer here contradicts the comment in enqueueSpan about immutable Process.
	sp.processSpan(sp.sanitizer(item.span), item.tenant)
	sp.metrics.InQueueLatency.Record(time.Since(item.queuedTime))
}

func (sp *spanProcessor) addCollectorTags(span *model.Span) {
	if len(sp.collectorTags) == 0 {
		return
	}
	dedupKey := make(map[string]struct{})
	for _, tag := range span.Process.Tags {
		if value, ok := sp.collectorTags[tag.Key]; ok && value == tag.AsString() {
			sp.logger.Debug("ignore collector process tags", zap.String("key", tag.Key), zap.String("value", value))
			dedupKey[tag.Key] = struct{}{}
		}
	}
	// ignore collector tags if has the same key-value in spans
	for k, v := range sp.collectorTags {
		if _, ok := dedupKey[k]; !ok {
			span.Process.Tags = append(span.Process.Tags, model.String(k, v))
		}
	}
	typedTags := model.KeyValues(span.Process.Tags)
	typedTags.Sort()
}

// Note: spans may share the Process object, so no changes should be made to Process
// in this function as it may cause race conditions.
func (sp *spanProcessor) enqueueSpan(span *model.Span, originalFormat processor.SpanFormat, transport processor.InboundTransport, tenant string) bool {
	spanCounts := sp.metrics.GetCountsForFormat(originalFormat, transport)
	spanCounts.ReceivedBySvc.ReportServiceNameForSpan(span)

	if !sp.filterSpan(span) {
		spanCounts.RejectedBySvc.ReportServiceNameForSpan(span)
		return true // as in "not dropped", because it's actively rejected
	}

	// add format tag
	span.Tags = append(span.Tags, model.String(jptrace.FormatAttribute, string(originalFormat)))

	item := queueItem{
		queuedTime: time.Now(),
		span:       span,
		tenant:     tenant,
	}
	return sp.queue.Produce(item)
}

func (sp *spanProcessor) background(reportPeriod time.Duration, callback func()) {
	go func() {
		ticker := time.NewTicker(reportPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				callback()
			case <-sp.stopCh:
				return
			}
		}
	}()
}

func (sp *spanProcessor) updateQueueSize() {
	if sp.dynQueueSizeWarmup == 0 {
		return
	}

	if sp.dynQueueSizeMemory == 0 {
		return
	}

	if sp.spansProcessed.Load() < uint64(sp.dynQueueSizeWarmup) {
		return
	}

	sp.queueResizeMu.Lock()
	defer sp.queueResizeMu.Unlock()

	// first, we get the average size of a span, by dividing the bytes processed by num of spans
	average := sp.bytesProcessed.Load() / sp.spansProcessed.Load()

	// finally, we divide the available memory by the average size of a span
	idealQueueSize := float64(sp.dynQueueSizeMemory / uint(average))

	// cap the queue size, just to be safe...
	if idealQueueSize > maxQueueSize {
		idealQueueSize = maxQueueSize
	}

	var diff float64
	current := float64(sp.queue.Capacity())
	if idealQueueSize > current {
		diff = idealQueueSize / current
	} else {
		diff = current / idealQueueSize
	}

	// resizing is a costly operation, we only perform it if we are at least n% apart from the desired value
	if diff > minRequiredChange {
		s := int(idealQueueSize)
		sp.logger.Info("Resizing the internal span queue", zap.Int("new-size", s), zap.Uint64("average-span-size-bytes", average))
		sp.queue.Resize(s)
	}
}

func (sp *spanProcessor) updateGauges() {
	//nolint: gosec // G115
	sp.metrics.SpansBytes.Update(int64(sp.bytesProcessed.Load()))
	sp.metrics.QueueLength.Update(int64(sp.queue.Size()))
	sp.metrics.QueueCapacity.Update(int64(sp.queue.Capacity()))
}
