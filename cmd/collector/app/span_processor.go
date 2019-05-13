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

	tchannel "github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/queue"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// ProcessSpansOptions additional options passed to processor along with the spans.
type ProcessSpansOptions struct {
	SpanFormat       SpanFormat
	InboundTransport InboundTransport
}

// SpanProcessor handles model spans
type SpanProcessor interface {
	// ProcessSpans processes model spans and return with either a list of true/false success or an error
	ProcessSpans(mSpans []*model.Span, options ProcessSpansOptions) ([]bool, error)
}

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

	sp.queue.StartLengthReporting(1*time.Second, sp.metrics.QueueLength)

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
		sp.metrics.SavedErrBySvc.ReportServiceNameForSpan(span)
	} else {
		sp.logger.Debug("Span written to the storage by the collector",
			zap.Stringer("trace-id", span.TraceID), zap.Stringer("span-id", span.SpanID))
		sp.metrics.SavedOkBySvc.ReportServiceNameForSpan(span)
	}
	sp.metrics.SaveLatency.Record(time.Since(startTime))
}

func (sp *spanProcessor) ProcessSpans(mSpans []*model.Span, options ProcessSpansOptions) ([]bool, error) {
	sp.preProcessSpans(mSpans)
	sp.metrics.BatchSize.Update(int64(len(mSpans)))
	retMe := make([]bool, len(mSpans))
	for i, mSpan := range mSpans {
		ok := sp.enqueueSpan(mSpan, options.SpanFormat, options.InboundTransport)
		if !ok && sp.reportBusy {
			return nil, tchannel.ErrServerBusy
		}
		retMe[i] = ok
	}
	return retMe, nil
}

func (sp *spanProcessor) processItemFromQueue(item *queueItem) {
	sp.processSpan(sp.sanitizer(item.span))
	sp.metrics.InQueueLatency.Record(time.Since(item.queuedTime))
}

func (sp *spanProcessor) enqueueSpan(span *model.Span, originalFormat SpanFormat, transport InboundTransport) bool {
	spanCounts := sp.metrics.GetCountsForFormat(originalFormat, transport)
	spanCounts.ReceivedBySvc.ReportServiceNameForSpan(span)

	if !sp.filterSpan(span) {
		spanCounts.RejectedBySvc.ReportServiceNameForSpan(span)
		return true // as in "not dropped", because it's actively rejected
	}

	//add format tag
	span.Tags = append(span.Tags, model.String("internal.span.format", string(originalFormat)))

	item := &queueItem{
		queuedTime: time.Now(),
		span:       span,
	}
	return sp.queue.Produce(item)
}
