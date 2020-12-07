// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/queue"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	// if this proves to be too low, we can increase it
	maxQueueSize = 1_000_000

	// if the new queue size isn't 20% bigger than the previous one, don't change
	minRequiredChange = 1.2
)

type spanProcessor struct {
	queue              *queue.BoundedQueue
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
	bytesProcessed     *atomic.Uint64
	spansProcessed     *atomic.Uint64
	stopCh             chan struct{}
}

type queueItem struct {
	queuedTime time.Time
	span       *model.Span
}

// NewSpanProcessor returns a SpanProcessor that preProcesses, filters, queues, sanitizes, and processes spans
func NewSpanProcessor(
	spanWriter spanstore.Writer,
	opts ...Option,
) processor.SpanProcessor {
	sp := newSpanProcessor(spanWriter, opts...)

	sp.queue.StartConsumers(sp.numWorkers, func(item interface{}) {
		value := item.(*queueItem)
		sp.processItemFromQueue(value)
	})

	sp.background(1*time.Second, sp.updateGauges)

	if sp.dynQueueSizeMemory > 0 {
		sp.background(1*time.Minute, sp.updateQueueSize)
	}

	return sp
}

func newSpanProcessor(spanWriter spanstore.Writer, opts ...Option) *spanProcessor {
	options := Options.apply(opts...)
	handlerMetrics := NewSpanProcessorMetrics(
		options.serviceMetrics,
		options.hostMetrics,
		options.extraFormatTypes)
	droppedItemHandler := func(item interface{}) {
		handlerMetrics.SpansDropped.Inc(1)
	}
	boundedQueue := queue.NewBoundedQueue(options.queueSize, droppedItemHandler)

	sp := spanProcessor{
		queue:              boundedQueue,
		metrics:            handlerMetrics,
		logger:             options.logger,
		preProcessSpans:    options.preProcessSpans,
		filterSpan:         options.spanFilter,
		sanitizer:          options.sanitizer,
		reportBusy:         options.reportBusy,
		numWorkers:         options.numWorkers,
		spanWriter:         spanWriter,
		collectorTags:      options.collectorTags,
		stopCh:             make(chan struct{}),
		dynQueueSizeMemory: options.dynQueueSizeMemory,
		dynQueueSizeWarmup: options.dynQueueSizeWarmup,
		bytesProcessed:     atomic.NewUint64(0),
		spansProcessed:     atomic.NewUint64(0),
	}

	processSpanFuncs := []ProcessSpan{options.preSave, sp.saveSpan}
	if options.dynQueueSizeMemory > 0 {
		// add to processSpanFuncs
		options.logger.Info("Dynamically adjusting the queue size at runtime.",
			zap.Uint("memory-mib", options.dynQueueSizeMemory/1024/1024),
			zap.Uint("queue-size-warmup", options.dynQueueSizeWarmup))
		processSpanFuncs = append(processSpanFuncs, sp.countSpan)
	}

	sp.processSpan = ChainedProcessSpan(processSpanFuncs...)
	return &sp
}

func (sp *spanProcessor) Close() error {
	close(sp.stopCh)
	sp.queue.Stop()

	return nil
}

func (sp *spanProcessor) saveSpan(span *model.Span) {
	if nil == span.Process {
		sp.logger.Error("process is empty for the span")
		sp.metrics.SavedErrBySvc.ReportServiceNameForSpan(span)
		return
	}

	startTime := time.Now()
	// TODO context should be propagated from upstream components
	if err := sp.spanWriter.WriteSpan(context.TODO(), span); err != nil {
		sp.logger.Error("Failed to save span", zap.Error(err))
		sp.metrics.SavedErrBySvc.ReportServiceNameForSpan(span)
	} else {
		sp.logger.Debug("Span written to the storage by the collector",
			zap.Stringer("trace-id", span.TraceID), zap.Stringer("span-id", span.SpanID))
		sp.metrics.SavedOkBySvc.ReportServiceNameForSpan(span)
	}
	sp.metrics.SaveLatency.Record(time.Since(startTime))
}

func (sp *spanProcessor) countSpan(span *model.Span) {
	sp.bytesProcessed.Add(uint64(span.Size()))
	sp.spansProcessed.Inc()
}

func (sp *spanProcessor) ProcessSpans(mSpans []*model.Span, options processor.SpansOptions) ([]bool, error) {
	sp.preProcessSpans(mSpans)
	sp.metrics.BatchSize.Update(int64(len(mSpans)))
	retMe := make([]bool, len(mSpans))
	for i, mSpan := range mSpans {
		ok := sp.enqueueSpan(mSpan, options.SpanFormat, options.InboundTransport)
		if !ok && sp.reportBusy {
			return nil, processor.ErrBusy
		}
		retMe[i] = ok
	}
	return retMe, nil
}

func (sp *spanProcessor) processItemFromQueue(item *queueItem) {
	sp.processSpan(sp.sanitizer(item.span))
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

func (sp *spanProcessor) enqueueSpan(span *model.Span, originalFormat processor.SpanFormat, transport processor.InboundTransport) bool {
	spanCounts := sp.metrics.GetCountsForFormat(originalFormat, transport)
	spanCounts.ReceivedBySvc.ReportServiceNameForSpan(span)

	if !sp.filterSpan(span) {
		spanCounts.RejectedBySvc.ReportServiceNameForSpan(span)
		return true // as in "not dropped", because it's actively rejected
	}

	//add format tag
	span.Tags = append(span.Tags, model.String("internal.span.format", string(originalFormat)))

	// append the collector tags
	sp.addCollectorTags(span)

	item := &queueItem{
		queuedTime: time.Now(),
		span:       span,
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
	sp.metrics.SpansBytes.Update(int64(sp.bytesProcessed.Load()))
	sp.metrics.QueueLength.Update(int64(sp.queue.Size()))
	sp.metrics.QueueCapacity.Update(int64(sp.queue.Capacity()))
}
