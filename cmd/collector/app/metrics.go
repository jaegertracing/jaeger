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
	"strings"
	"sync"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/model"
)

const (
	// TODO this needs to be configurable via CLI.
	maxServiceNames = 4000

	// otherServices is the catch-all label when number of services exceeds maxServiceNames
	otherServices = "other-services"

	samplerTypeKey           = "sampler_type"
	samplerTypeConst         = "const"
	samplerTypeRemote        = "remote"
	samplerTypeProbabilistic = "probabilistic"
	samplerTypeRateLimiting  = "ratelimiting"
	samplerTypeLowerBound    = "lowerbound"
	samplerTypeUnknown       = "unknown"
	// types of samplers: const, remote, probabilistic, ratelimiting, lowerbound
	numOfSamplerTypes = 5

	concatenation = "$_$"

	otherServicesConstSampler         = otherServices + concatenation + samplerTypeConst
	otherServicesRemoteSampler        = otherServices + concatenation + samplerTypeRemote
	otherServicesProbabilisticSampler = otherServices + concatenation + samplerTypeProbabilistic
	otherServicesRateLimitingSampler  = otherServices + concatenation + samplerTypeRateLimiting
	otherServicesLowerBoundSampler    = otherServices + concatenation + samplerTypeLowerBound
	otherServicesUnknownSampler       = otherServices + concatenation + samplerTypeUnknown
)

// SpanProcessorMetrics contains all the necessary metrics for the SpanProcessor
type SpanProcessorMetrics struct {
	//TODO - initialize metrics in the traditional factory way. Initialize map afterward.
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
	// SavedOkBySvc contains span and trace counts by service
	SavedOkBySvc  metricsBySvc  // spans actually saved
	SavedErrBySvc metricsBySvc  // spans failed to save
	serviceNames  metrics.Gauge // total number of unique service name metrics reported by this collector
	spanCounts    SpanCountsByFormat
}

type spanCountsBySvc struct {
	counts          map[string]metrics.Counter // counters per service
	debugCounts     map[string]metrics.Counter // debug counters per service
	factory         metrics.Factory
	lock            *sync.Mutex
	maxServiceNames int
	category        string
}

type traceCountsBySvc struct {
	counts            map[string]metrics.Counter // counters per service
	debugCounts       map[string]metrics.Counter // debug counters per service
	factory           metrics.Factory
	lock              *sync.Mutex
	maxServices       int // servicesNames * samplerTypes
	category          string
	stringBuilderPool *sync.Pool
}

type metricsBySvc struct {
	spans  spanCountsBySvc  // number of spans received per service
	traces traceCountsBySvc // number of traces originated per service
}

// InboundTransport identifies the transport used to receive spans.
type InboundTransport string

const (
	// GRPCTransport indicates spans received over gRPC.
	GRPCTransport InboundTransport = "grpc"
	// TChannelTransport indicates spans received over TChannel.
	TChannelTransport InboundTransport = "tchannel"
	// HTTPTransport indicates spans received over HTTP.
	HTTPTransport InboundTransport = "http"
	// UnknownTransport is the fallback/catch-all category.
	UnknownTransport InboundTransport = "unknown"
)

// SpanFormat identifies the data format in which the span was originally received.
type SpanFormat string

const (
	// JaegerSpanFormat is for Jaeger Thrift spans.
	JaegerSpanFormat SpanFormat = "jaeger"
	// ZipkinSpanFormat is for Zipkin Thrift spans.
	ZipkinSpanFormat SpanFormat = "zipkin"
	// ProtoSpanFormat is for Jaeger protobuf Spans.
	ProtoSpanFormat SpanFormat = "proto"
	// UnknownSpanFormat is the fallback/catch-all category.
	UnknownSpanFormat SpanFormat = "unknown"
)

// SpanCountsByFormat groups metrics by different span formats (thrift, proto, etc.)
type SpanCountsByFormat map[SpanFormat]SpanCountsByTransport

// SpanCountsByTransport groups metrics by inbound transport (e.g http, grpc, tchannel)
type SpanCountsByTransport map[InboundTransport]SpanCounts

// SpanCounts contains countrs for received and rejected spans.
type SpanCounts struct {
	// ReceivedBySvc maintain by-service metrics.
	ReceivedBySvc metricsBySvc
	// RejectedBySvc is the number of spans we rejected (usually due to blacklisting) by-service.
	RejectedBySvc metricsBySvc
}

// NewSpanProcessorMetrics returns a SpanProcessorMetrics
func NewSpanProcessorMetrics(serviceMetrics metrics.Factory, hostMetrics metrics.Factory, otherFormatTypes []SpanFormat) *SpanProcessorMetrics {
	spanCounts := SpanCountsByFormat{
		ZipkinSpanFormat:  newCountsByTransport(serviceMetrics, ZipkinSpanFormat),
		JaegerSpanFormat:  newCountsByTransport(serviceMetrics, JaegerSpanFormat),
		ProtoSpanFormat:   newCountsByTransport(serviceMetrics, ProtoSpanFormat),
		UnknownSpanFormat: newCountsByTransport(serviceMetrics, UnknownSpanFormat),
	}
	for _, otherFormatType := range otherFormatTypes {
		spanCounts[otherFormatType] = newCountsByTransport(serviceMetrics, otherFormatType)
	}
	m := &SpanProcessorMetrics{
		SaveLatency:    hostMetrics.Timer(metrics.TimerOptions{Name: "save-latency", Tags: nil}),
		InQueueLatency: hostMetrics.Timer(metrics.TimerOptions{Name: "in-queue-latency", Tags: nil}),
		SpansDropped:   hostMetrics.Counter(metrics.Options{Name: "spans.dropped", Tags: nil}),
		BatchSize:      hostMetrics.Gauge(metrics.Options{Name: "batch-size", Tags: nil}),
		QueueLength:    hostMetrics.Gauge(metrics.Options{Name: "queue-length", Tags: nil}),
		SavedOkBySvc:   newMetricsBySvc(serviceMetrics.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"result": "ok"}}), "saved-by-svc"),
		SavedErrBySvc:  newMetricsBySvc(serviceMetrics.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"result": "err"}}), "saved-by-svc"),
		spanCounts:     spanCounts,
		serviceNames:   hostMetrics.Gauge(metrics.Options{Name: "spans.serviceNames", Tags: nil}),
	}

	return m
}

func newMetricsBySvc(factory metrics.Factory, category string) metricsBySvc {
	spansFactory := factory.Namespace(metrics.NSOptions{Name: "spans", Tags: nil})
	tracesFactory := factory.Namespace(metrics.NSOptions{Name: "traces", Tags: nil})
	return metricsBySvc{
		spans:  newSpanCountsBySvc(spansFactory, category, maxServiceNames),
		traces: newTraceCountsBySvc(tracesFactory, category, maxServiceNames),
	}
}

func newTraceCountsBySvc(factory metrics.Factory, category string, maxServices int) traceCountsBySvc {
	return traceCountsBySvc{
		counts:      newTraceCountsOtherServices(factory, category, "false"),
		debugCounts: newTraceCountsOtherServices(factory, category, "true"),
		factory:     factory,
		lock:        &sync.Mutex{},
		maxServices: maxServices + numOfSamplerTypes, // numOfSamplerType is the offset added to maxServices threshold
		category:    category,
		// use sync.Pool to reduce allocation of stringBuilder
		stringBuilderPool: &sync.Pool{
			New: func() interface{} {
				return new(strings.Builder)
			},
		},
	}
}

func newTraceCountsOtherServices(factory metrics.Factory, category string, isDebug string) map[string]metrics.Counter {
	return map[string]metrics.Counter{
		otherServicesConstSampler:         factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": isDebug, samplerTypeKey: samplerTypeConst}}),
		otherServicesLowerBoundSampler:    factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": isDebug, samplerTypeKey: samplerTypeLowerBound}}),
		otherServicesProbabilisticSampler: factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": isDebug, samplerTypeKey: samplerTypeProbabilistic}}),
		otherServicesRateLimitingSampler:  factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": isDebug, samplerTypeKey: samplerTypeRateLimiting}}),
		otherServicesRemoteSampler:        factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": isDebug, samplerTypeKey: samplerTypeRemote}}),
		otherServicesUnknownSampler:       factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": isDebug, samplerTypeKey: samplerTypeUnknown}}),
	}
}

func newSpanCountsBySvc(factory metrics.Factory, category string, maxServiceNames int) spanCountsBySvc {
	return spanCountsBySvc{
		counts:          map[string]metrics.Counter{otherServices: factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": "false"}})},
		debugCounts:     map[string]metrics.Counter{otherServices: factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": "true"}})},
		factory:         factory,
		lock:            &sync.Mutex{},
		maxServiceNames: maxServiceNames,
		category:        category,
	}
}

func newCountsByTransport(factory metrics.Factory, format SpanFormat) SpanCountsByTransport {
	factory = factory.Namespace(metrics.NSOptions{Tags: map[string]string{"format": string(format)}})
	return SpanCountsByTransport{
		HTTPTransport:     newCounts(factory, HTTPTransport),
		TChannelTransport: newCounts(factory, TChannelTransport),
		GRPCTransport:     newCounts(factory, GRPCTransport),
		UnknownTransport:  newCounts(factory, UnknownTransport),
	}
}

func newCounts(factory metrics.Factory, transport InboundTransport) SpanCounts {
	factory = factory.Namespace(metrics.NSOptions{Tags: map[string]string{"transport": string(transport)}})
	return SpanCounts{
		RejectedBySvc: newMetricsBySvc(factory, "rejected"),
		ReceivedBySvc: newMetricsBySvc(factory, "received"),
	}
}

// GetCountsForFormat gets the SpanCounts for a given format and transport. If none exists, we use the Unknown format.
func (m *SpanProcessorMetrics) GetCountsForFormat(spanFormat SpanFormat, transport InboundTransport) SpanCounts {
	c, ok := m.spanCounts[spanFormat]
	if !ok {
		c = m.spanCounts[UnknownSpanFormat]
	}
	t, ok := c[transport]
	if !ok {
		t = c[UnknownTransport]
	}
	return t
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

		m.countTracesByServiceName(serviceName, span.Flags.IsDebug(), span.
			GetSamplerType())
	}
}

// countSpansByServiceName counts how many spans are received per service.
func (m metricsBySvc) countSpansByServiceName(serviceName string, isDebug bool) {
	m.spans.countByServiceName(serviceName, isDebug)
}

// countTracesByServiceName counts how many traces are received per service,
// i.e. the counter is only incremented for the root spans.
func (m metricsBySvc) countTracesByServiceName(serviceName string, isDebug bool, samplerType string) {
	m.traces.countByServiceName(serviceName, isDebug, samplerType)
}

// traceCountsBySvc.countByServiceName maintains a map of counters for each service name it's
// given and increments the respective counter when called. The service name
// are first normalized to safe-for-metrics format.  If the number of counters
// exceeds maxServiceNames, new service names are ignored to avoid polluting
// the metrics namespace and overloading M3.
//
// The reportServiceNameCount() function runs on a timer and will report the
// total number of stored counters, so if it exceeds say the 90% threshold
// an alert should be raised to investigate what's causing so many unique
// service names.
func (m *traceCountsBySvc) countByServiceName(serviceName string, isDebug bool, samplerType string) {
	serviceName = NormalizeServiceName(serviceName)
	counts := m.counts
	if isDebug {
		counts = m.debugCounts
	}
	var counter metrics.Counter
	m.lock.Lock()

	// trace counter key is combination of serviceName and samplerType.
	key := m.buildKey(serviceName, samplerType)

	if c, ok := counts[key]; ok {
		counter = c
	} else if len(counts) < m.maxServices {
		debugStr := "false"
		if isDebug {
			debugStr = "true"
		}
		// Only trace metrics have samplerType tag
		tags := map[string]string{"svc": serviceName, "debug": debugStr, samplerTypeKey: samplerType}

		c := m.factory.Counter(metrics.Options{Name: m.category, Tags: tags})
		counts[key] = c
		counter = c
	} else {
		switch samplerType {
		case samplerTypeConst:
			counter = counts[otherServicesConstSampler]
		case samplerTypeRemote:
			counter = counts[otherServicesRemoteSampler]
		case samplerTypeLowerBound:
			counter = counts[otherServicesLowerBoundSampler]
		case samplerTypeProbabilistic:
			counter = counts[otherServicesProbabilisticSampler]
		case samplerTypeRateLimiting:
			counter = counts[otherServicesRateLimitingSampler]
		default:
			counter = counts[otherServicesUnknownSampler]
		}
	}
	m.lock.Unlock()
	counter.Inc(1)
}

// spanCountsBySvc.countByServiceName maintains a map of counters for each service name it's
// given and increments the respective counter when called. The service name
// are first normalized to safe-for-metrics format.  If the number of counters
// exceeds maxServiceNames, new service names are ignored to avoid polluting
// the metrics namespace and overloading M3.
//
// The reportServiceNameCount() function runs on a timer and will report the
// total number of stored counters, so if it exceeds say the 90% threshold
// an alert should be raised to investigate what's causing so many unique
// service names.
func (m *spanCountsBySvc) countByServiceName(serviceName string, isDebug bool) {
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
		tags := map[string]string{"svc": serviceName, "debug": debugStr}
		c := m.factory.Counter(metrics.Options{Name: m.category, Tags: tags})
		counts[serviceName] = c
		counter = c
	} else {
		counter = counts[otherServices]
	}
	m.lock.Unlock()
	counter.Inc(1)
}

func (m *traceCountsBySvc) buildKey(serviceName, samplerType string) string {
	keyBuilder := m.stringBuilderPool.Get().(*strings.Builder)
	keyBuilder.Reset()
	keyBuilder.WriteString(serviceName)
	keyBuilder.WriteString(concatenation)
	keyBuilder.WriteString(samplerType)
	key := keyBuilder.String()
	m.stringBuilderPool.Put(keyBuilder)
	return key
}
