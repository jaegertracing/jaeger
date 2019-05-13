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
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/model"
)

const (
	// DefaultNumWorkers is the default number of workers consuming from the processor queue
	DefaultNumWorkers = 50
	// DefaultQueueSize is the size of the processor's queue
	DefaultQueueSize = 2000
)

type options struct {
	logger           *zap.Logger
	serviceMetrics   metrics.Factory
	hostMetrics      metrics.Factory
	preProcessSpans  ProcessSpans
	sanitizer        sanitizer.SanitizeSpan
	preSave          ProcessSpan
	spanFilter       FilterSpan
	numWorkers       int
	blockingSubmit   bool
	queueSize        int
	reportBusy       bool
	extraFormatTypes []SpanFormat
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

// PreProcessSpans creates an Option that initializes the preProcessSpans function
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

// ReportBusy creates an Option that initializes the reportBusy boolean
func (options) ReportBusy(reportBusy bool) Option {
	return func(b *options) {
		b.reportBusy = reportBusy
	}
}

// ExtraFormatTypes creates an Option that initializes the extra list of format types
func (options) ExtraFormatTypes(extraFormatTypes []SpanFormat) Option {
	return func(b *options) {
		b.extraFormatTypes = extraFormatTypes
	}
}

func (o options) apply(opts ...Option) options {
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
		ret.preProcessSpans = func(spans []*model.Span) {}
	}
	if ret.sanitizer == nil {
		ret.sanitizer = func(span *model.Span) *model.Span { return span }
	}
	if ret.preSave == nil {
		ret.preSave = func(span *model.Span) {}
	}
	if ret.spanFilter == nil {
		ret.spanFilter = func(span *model.Span) bool { return true }
	}
	if ret.numWorkers == 0 {
		ret.numWorkers = DefaultNumWorkers
	}
	return ret
}
