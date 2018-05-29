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
	"sync"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/model"
)

const (
	maxServiceNames = 2000
	otherServices   = "other-services"
)

// SpanProcessorMetrics contains all the necessary metrics for the SpanProcessor
type SpanProcessorMetrics struct { //TODO - initialize metrics in the traditional factory way. Initialize map afterward.
	// SaveLatency measures how long the actual save to storage takes
	SaveLatency metrics.Timer
	// InQueueLatency measures how long the span spends in the queue
	InQueueLatency metrics.Timer
	// SpansDropped measures the number of spans we discarded because the queue was full
	SpansDropped metrics.Counter
	// BatchSize measures the span batch size
	BatchSize metrics.Gauge // size of span batch
	// QueueLength measures the size of the internal span queue
	QueueLength metrics.Gauge
	// ErrorBusy counts number of return ErrServerBusy
	ErrorBusy metrics.Counter
	// SavedBySvc contains span and trace counts by service
	SavedBySvc   metricsBySvc  // spans actually saved
	serviceNames metrics.Gauge // total number of unique service name metrics reported by this collector
	spanCounts   map[string]CountsBySpanType
}

type countsBySvc struct {
	counts          map[string]metrics.Counter // counters per service
	debugCounts     map[string]metrics.Counter // debug counters per service
	factory         metrics.Factory
	lock            *sync.Mutex
	maxServiceNames int
}

type metricsBySvc struct {
	spans  countsBySvc // number of spans received per service
	traces countsBySvc // number of traces originated per service
}

// CountsBySpanType measures received, rejected, and receivedByService metrics for a format type
type CountsBySpanType struct {
	// ReceivedBySvc maintain by-service metrics for a format type
	ReceivedBySvc metricsBySvc
	// RejectedBySvc is the number of spans we rejected (usually due to blacklisting) by-service
	RejectedBySvc metricsBySvc
}

// NewSpanProcessorMetrics returns a SpanProcessorMetrics
func NewSpanProcessorMetrics(serviceMetrics metrics.Factory, hostMetrics metrics.Factory, otherFormatTypes []string) *SpanProcessorMetrics {
	spanCounts := map[string]CountsBySpanType{
		ZipkinFormatType:  newCountsBySpanType(serviceMetrics.Namespace("", map[string]string{"format": ZipkinFormatType})),
		JaegerFormatType:  newCountsBySpanType(serviceMetrics.Namespace("", map[string]string{"format": JaegerFormatType})),
		UnknownFormatType: newCountsBySpanType(serviceMetrics.Namespace("", map[string]string{"format": UnknownFormatType})),
	}
	for _, otherFormatType := range otherFormatTypes {
		spanCounts[otherFormatType] = newCountsBySpanType(serviceMetrics.Namespace("", map[string]string{"format": otherFormatType}))
	}
	m := &SpanProcessorMetrics{
		SaveLatency:    hostMetrics.Timer("save-latency", nil),
		InQueueLatency: hostMetrics.Timer("in-queue-latency", nil),
		SpansDropped:   hostMetrics.Counter("spans.dropped", nil),
		BatchSize:      hostMetrics.Gauge("batch-size", nil),
		QueueLength:    hostMetrics.Gauge("queue-length", nil),
		ErrorBusy:      hostMetrics.Counter("error.busy", nil),
		SavedBySvc:     newMetricsBySvc(serviceMetrics, "saved-by-svc"),
		spanCounts:     spanCounts,
		serviceNames:   hostMetrics.Gauge("spans.serviceNames", nil),
	}

	return m
}

func newMetricsBySvc(factory metrics.Factory, category string) metricsBySvc {
	spansFactory := factory.Namespace("spans."+category, nil)
	tracesFactory := factory.Namespace("traces."+category, nil)
	return metricsBySvc{
		spans:  newCountsBySvc(spansFactory, maxServiceNames),
		traces: newCountsBySvc(tracesFactory, maxServiceNames),
	}
}

func newCountsBySvc(factory metrics.Factory, maxServiceNames int) countsBySvc {
	return countsBySvc{
		counts: map[string]metrics.Counter{
			otherServices: factory.Counter("", map[string]string{"service": otherServices, "debug": "false"}),
		},
		debugCounts: map[string]metrics.Counter{
			otherServices: factory.Counter("", map[string]string{"service": otherServices, "debug": "true"}),
		},
		factory:         factory,
		lock:            &sync.Mutex{},
		maxServiceNames: maxServiceNames,
	}
}

func newCountsBySpanType(factory metrics.Factory) CountsBySpanType {
	return CountsBySpanType{
		RejectedBySvc: newMetricsBySvc(factory, "rejected"),
		ReceivedBySvc: newMetricsBySvc(factory, "received"),
	}
}

// GetCountsForFormat gets the countsBySpanType for a given format. If none exists, we use the Unknown format.
func (m *SpanProcessorMetrics) GetCountsForFormat(spanFormat string) CountsBySpanType {
	c, ok := m.spanCounts[spanFormat]
	if !ok {
		return m.spanCounts[UnknownFormatType]
	}
	return c
}

// reportServiceNameForSpan determines the name of the service that emitted
// the span and reports a counter stat.
func (m metricsBySvc) ReportServiceNameForSpan(span *model.Span) {
	serviceName := span.Process.ServiceName
	if serviceName == "" {
		return
	}
	m.countSpansByServiceName(serviceName, span.Flags.IsDebug())
	if span.ParentSpanID() == 0 {
		m.countTracesByServiceName(serviceName, span.Flags.IsDebug())
	}
}

// countSpansByServiceName counts how many spans are received per service.
func (m metricsBySvc) countSpansByServiceName(serviceName string, isDebug bool) {
	m.spans.countByServiceName(serviceName, isDebug)
}

// countTracesByServiceName counts how many traces are received per service,
// i.e. the counter is only incremented for the root spans.
func (m metricsBySvc) countTracesByServiceName(serviceName string, isDebug bool) {
	m.traces.countByServiceName(serviceName, isDebug)
}

// countByServiceName maintains a map of counters for each service name it's
// given and increments the respective counter when called. The service name
// are first normalized to safe-for-metrics format.  If the number of counters
// exceeds maxServiceNames, new service names are ignored to avoid polluting
// the metrics namespace and overloading M3.
//
// The reportServiceNameCount() function runs on a timer and will report the
// total number of stored counters, so if it exceeds say the 90% threshold
// an alert should be raised to investigate what's causing so many unique
// service names.
func (m *countsBySvc) countByServiceName(serviceName string, isDebug bool) {
	serviceName = NormalizeServiceName(serviceName)
	counts := m.counts
	if isDebug {
		counts = m.debugCounts
	}
	var counter metrics.Counter
	m.lock.Lock()
	if c, ok := counts[serviceName]; ok {
		counter = c
	} else if len(counts) < m.maxServiceNames {
		debugStr := "false"
		if isDebug {
			debugStr = "true"
		}
		c := m.factory.Counter("", map[string]string{"service": serviceName, "debug": debugStr})
		counts[serviceName] = c
		counter = c
	} else {
		counter = counts[otherServices]
	}
	m.lock.Unlock()
	counter.Inc(1)
}
