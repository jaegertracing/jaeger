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
	"time"

	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"

	"github.com/uber/jaeger/cmd/collector/app/sanitizer"
	"github.com/uber/jaeger/pkg/queue"
)

type spanProcessor struct {
	queue           *queue.BoundedQueue
	metrics         *SpanProcessorMetrics
	preProcessSpans ProcessSpans
	filterSpan      FilterSpan             // filter is called before the sanitizer but after preProcessSpans
	sanitizer       sanitizer.SanitizeSpan // sanitizer is called before processSpan
	processSpan     ProcessSpan
	logger          *zap.Logger
	spanWriter      spanstore.Writer
	reportBusy      bool
	numWorkers      int
}

type queueItem struct {
	queuedTime time.Time
	span       *model.Span
}

// NewSpanProcessor returns a SpanProcessor that preProcesses, filters, queues, sanitizes, and processes spans
func NewSpanProcessor(
	spanWriter spanstore.Writer,
	opts ...Option,
) SpanProcessor {
	sp := newSpanProcessor(spanWriter, opts...)

	sp.queue.StartConsumers(sp.numWorkers, func(item interface{}) {
		value := item.(*queueItem)
		sp.processItemFromQueue(value)
	})

	sp.queue.StartLengthReporting(10*time.Second, sp.metrics.QueueLength)

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
		handlerMetrics.QueueLength.Update(int64(options.queueSize))
	}
	boundedQueue := queue.NewBoundedQueue(options.queueSize, droppedItemHandler)

	sp := spanProcessor{
		queue:           boundedQueue,
		metrics:         handlerMetrics,
		logger:          options.logger,
		preProcessSpans: options.preProcessSpans,
		filterSpan:      options.spanFilter,
		sanitizer:       options.sanitizer,
		reportBusy:      options.reportBusy,
		numWorkers:      options.numWorkers,
		spanWriter:      spanWriter,
	}
	sp.processSpan = ChainedProcessSpan(
		options.preSave,
		sp.saveSpan,
	)

	return &sp
}

// Stop halts the span processor and all its go-routines.
func (sp *spanProcessor) Stop() {
	sp.queue.Stop()
}

func (sp *spanProcessor) saveSpan(span *model.Span) {
	startTime := time.Now()
	if err := sp.spanWriter.WriteSpan(span); err != nil {
		sp.logger.Error("Failed to save span", zap.Error(err))
	} else {
		sp.metrics.SavedBySvc.ReportServiceNameForSpan(span)
	}
	sp.metrics.SaveLatency.Record(time.Now().Sub(startTime))
}

func (sp *spanProcessor) ProcessSpans(mSpans []*model.Span, spanFormat string) ([]bool, error) {
	sp.preProcessSpans(mSpans)
	sp.metrics.GetCountsForFormat(spanFormat).Received.Inc(int64(len(mSpans)))
	sp.metrics.BatchSize.Update(int64(len(mSpans)))
	retMe := make([]bool, len(mSpans))
	for i, mSpan := range mSpans {
		ok := sp.enqueueSpan(mSpan, spanFormat)
		if !ok && sp.reportBusy {
			return nil, tchannel.ErrServerBusy
		}
		retMe[i] = ok
	}
	return retMe, nil
}

func (sp *spanProcessor) processItemFromQueue(item *queueItem) {
	sp.processSpan(sp.sanitizer(item.span))
	sp.metrics.InQueueLatency.Record(time.Now().Sub(item.queuedTime))
}

func (sp *spanProcessor) enqueueSpan(span *model.Span, originalFormat string) bool {
	spanCounts := sp.metrics.GetCountsForFormat(originalFormat)
	spanCounts.ReceivedBySvc.ReportServiceNameForSpan(span)

	if !sp.filterSpan(span) {
		spanCounts.Rejected.Inc(int64(1))
		return true // as in "not dropped", because it's actively rejected
	}
	item := &queueItem{
		queuedTime: time.Now(),
		span:       span,
	}
	addedToQueue := sp.queue.Produce(item)
	if !addedToQueue {
		sp.metrics.ErrorBusy.Inc(1)
	}
	return addedToQueue
}
