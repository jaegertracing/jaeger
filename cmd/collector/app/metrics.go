// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/normalizer"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

const (
	// TODO this needs to be configurable via CLI.
	maxServiceNames = 4000

	// otherServices is the catch-all label when number of services exceeds maxServiceNames
	otherServices = "other-services"

	// samplerTypeKey is the name of the metric tag showing sampler type
	samplerTypeKey = "sampler_type"

	concatenation = "$_$"

	// unknownServiceName is used when a span has no service name
	unknownServiceName = "__unknown"
)

var otherServicesSamplers map[model.SamplerType]string = initOtherServicesSamplers()

func initOtherServicesSamplers() map[model.SamplerType]string {
	samplers := []model.SamplerType{
		model.SamplerTypeUnrecognized,
		model.SamplerTypeProbabilistic,
		model.SamplerTypeLowerBound,
		model.SamplerTypeRateLimiting,
		model.SamplerTypeConst,
	}
	m := make(map[model.SamplerType]string)
	for _, s := range samplers {
		m[s] = otherServices + concatenation + s.String()
	}
	return m
}

// SpanProcessorMetrics contains all the necessary metrics for the SpanProcessor
type SpanProcessorMetrics struct {
	// TODO - initialize metrics in the traditional factory way. Initialize map afterward.
	// SaveLatency measures how long the actual save to storage takes
	SaveLatency metrics.Timer
	// InQueueLatency measures how long the span spends in the queue
	InQueueLatency metrics.Timer
	// SpansDropped measures the number of spans we discarded because the queue was full
	SpansDropped metrics.Counter
	// SpansBytes records how many bytes were processed
	SpansBytes metrics.Gauge
	// BatchSize measures the span batch size
	BatchSize metrics.Gauge // size of span batch
	// QueueCapacity measures the capacity of the internal span queue
	QueueCapacity metrics.Gauge
	// QueueLength measures the current number of elements in the internal span queue
	QueueLength metrics.Gauge
	// SavedOkBySvc contains span and trace counts by service
	SavedOkBySvc  metricsBySvc  // spans actually saved
	SavedErrBySvc metricsBySvc  // spans failed to save
	serviceNames  metrics.Gauge // total number of unique service name metrics reported by this collector
	spanCounts    SpanCountsByFormat
}

type countsBySvc struct {
	counts          map[string]metrics.Counter // counters per service
	debugCounts     map[string]metrics.Counter // debug counters per service
	factory         metrics.Factory
	lock            *sync.Mutex
	maxServiceNames int
	category        string
}

type spanCountsBySvc struct {
	countsBySvc
}

type traceCountsBySvc struct {
	countsBySvc
	stringBuilderPool *sync.Pool
}

type metricsBySvc struct {
	spans  spanCountsBySvc  // number of spans received per service
	traces traceCountsBySvc // number of traces originated per service
}

// SpanCountsByFormat groups metrics by different span formats (thrift, proto, etc.)
type SpanCountsByFormat map[processor.SpanFormat]SpanCountsByTransport

// SpanCountsByTransport groups metrics by inbound transport (e.g http, grpc, tchannel)
type SpanCountsByTransport map[processor.InboundTransport]SpanCounts

// SpanCounts contains counts for received and rejected spans.
type SpanCounts struct {
	// ReceivedBySvc maintain by-service metrics.
	ReceivedBySvc metricsBySvc
	// RejectedBySvc is the number of spans we rejected (usually due to blacklisting) by-service.
	RejectedBySvc metricsBySvc
}

// NewSpanProcessorMetrics returns a SpanProcessorMetrics
func NewSpanProcessorMetrics(serviceMetrics metrics.Factory, hostMetrics metrics.Factory, otherFormatTypes []processor.SpanFormat) *SpanProcessorMetrics {
	spanCounts := SpanCountsByFormat{
		processor.ZipkinSpanFormat:  newCountsByTransport(serviceMetrics, processor.ZipkinSpanFormat),
		processor.JaegerSpanFormat:  newCountsByTransport(serviceMetrics, processor.JaegerSpanFormat),
		processor.ProtoSpanFormat:   newCountsByTransport(serviceMetrics, processor.ProtoSpanFormat),
		processor.UnknownSpanFormat: newCountsByTransport(serviceMetrics, processor.UnknownSpanFormat),
	}
	for _, otherFormatType := range otherFormatTypes {
		spanCounts[otherFormatType] = newCountsByTransport(serviceMetrics, otherFormatType)
	}
	m := &SpanProcessorMetrics{
		SaveLatency:    hostMetrics.Timer(metrics.TimerOptions{Name: "save-latency", Tags: nil}),
		InQueueLatency: hostMetrics.Timer(metrics.TimerOptions{Name: "in-queue-latency", Tags: nil}),
		SpansDropped:   hostMetrics.Counter(metrics.Options{Name: "spans.dropped", Tags: nil}),
		BatchSize:      hostMetrics.Gauge(metrics.Options{Name: "batch-size", Tags: nil}),
		QueueCapacity:  hostMetrics.Gauge(metrics.Options{Name: "queue-capacity", Tags: nil}),
		QueueLength:    hostMetrics.Gauge(metrics.Options{Name: "queue-length", Tags: nil}),
		SpansBytes:     hostMetrics.Gauge(metrics.Options{Name: "spans.bytes", Tags: nil}),
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
	extraSlotsForOtherServicesSamples := len(otherServicesSamplers) - 1 // excluding UnrecognizedSampler
	return traceCountsBySvc{
		countsBySvc: countsBySvc{
			counts:          newTraceCountsOtherServices(factory, category, "false"),
			debugCounts:     newTraceCountsOtherServices(factory, category, "true"),
			factory:         factory,
			lock:            &sync.Mutex{},
			maxServiceNames: maxServices + extraSlotsForOtherServicesSamples,
			category:        category,
		},
		// use sync.Pool to reduce allocation of stringBuilder
		stringBuilderPool: &sync.Pool{
			New: func() any {
				return new(strings.Builder)
			},
		},
	}
}

func newTraceCountsOtherServices(factory metrics.Factory, category string, isDebug string) map[string]metrics.Counter {
	m := make(map[string]metrics.Counter)
	for kSampler, vString := range otherServicesSamplers {
		m[vString] = factory.Counter(
			metrics.Options{
				Name: category,
				Tags: map[string]string{
					"svc":          otherServices,
					"debug":        isDebug,
					samplerTypeKey: kSampler.String(),
				},
			})
	}
	return m
}

func newSpanCountsBySvc(factory metrics.Factory, category string, maxServiceNames int) spanCountsBySvc {
	return spanCountsBySvc{
		countsBySvc: countsBySvc{
			counts:          map[string]metrics.Counter{otherServices: factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": "false"}})},
			debugCounts:     map[string]metrics.Counter{otherServices: factory.Counter(metrics.Options{Name: category, Tags: map[string]string{"svc": otherServices, "debug": "true"}})},
			factory:         factory,
			lock:            &sync.Mutex{},
			maxServiceNames: maxServiceNames,
			category:        category,
		},
	}
}

func newCountsByTransport(factory metrics.Factory, format processor.SpanFormat) SpanCountsByTransport {
	factory = factory.Namespace(metrics.NSOptions{Tags: map[string]string{"format": string(format)}})
	return SpanCountsByTransport{
		processor.HTTPTransport:    newCounts(factory, processor.HTTPTransport),
		processor.GRPCTransport:    newCounts(factory, processor.GRPCTransport),
		processor.UnknownTransport: newCounts(factory, processor.UnknownTransport),
	}
}

func newCounts(factory metrics.Factory, transport processor.InboundTransport) SpanCounts {
	factory = factory.Namespace(metrics.NSOptions{Tags: map[string]string{"transport": string(transport)}})
	return SpanCounts{
		RejectedBySvc: newMetricsBySvc(factory, "rejected"),
		ReceivedBySvc: newMetricsBySvc(factory, "received"),
	}
}

// GetCountsForFormat gets the SpanCounts for a given format and transport. If none exists, we use the Unknown format.
func (m *SpanProcessorMetrics) GetCountsForFormat(spanFormat processor.SpanFormat, transport processor.InboundTransport) SpanCounts {
	c, ok := m.spanCounts[spanFormat]
	if !ok {
		c = m.spanCounts[processor.UnknownSpanFormat]
	}
	t, ok := c[transport]
	if !ok {
		t = c[processor.UnknownTransport]
	}
	return t
}

// ForSpanV1 determines the name of the service that emitted
// the span and reports a counter stat.
func (m metricsBySvc) ForSpanV1(span *model.Span) {
	var serviceName string
	if nil == span.Process || len(span.Process.ServiceName) == 0 {
		serviceName = unknownServiceName
	} else {
		serviceName = span.Process.ServiceName
	}

	m.countSpansByServiceName(serviceName, span.Flags.IsDebug())
	if span.ParentSpanID() == 0 {
		m.countTracesByServiceName(serviceName, span.Flags.IsDebug(), span.
			GetSamplerType())
	}
}

// ForSpanV2 determines the name of the service that emitted
// the span and reports a counter stat.
func (m metricsBySvc) ForSpanV2(resource pcommon.Resource, span ptrace.Span) {
	serviceName := unknownServiceName
	if v, ok := resource.Attributes().Get(string(otelsemconv.ServiceNameKey)); ok {
		serviceName = v.AsString()
	}

	m.countSpansByServiceName(serviceName, false)
	if span.ParentSpanID().IsEmpty() {
		m.countTracesByServiceName(serviceName, false, model.SamplerTypeUnrecognized)
	}
}

// countSpansByServiceName counts how many spans are received per service.
func (m metricsBySvc) countSpansByServiceName(serviceName string, isDebug bool) {
	m.spans.countByServiceName(serviceName, isDebug)
}

// countTracesByServiceName counts how many traces are received per service,
// i.e. the counter is only incremented for the root spans.
func (m metricsBySvc) countTracesByServiceName(serviceName string, isDebug bool, samplerType model.SamplerType) {
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
func (m *traceCountsBySvc) countByServiceName(serviceName string, isDebug bool, samplerType model.SamplerType) {
	serviceName = normalizer.ServiceName(serviceName)
	counts := m.counts
	if isDebug {
		counts = m.debugCounts
	}
	var counter metrics.Counter
	m.lock.Lock()

	// trace counter key is combination of serviceName and samplerType.
	key := m.buildKey(serviceName, samplerType.String())

	if c, ok := counts[key]; ok {
		counter = c
	} else if len(counts) < m.maxServiceNames {
		debugStr := "false"
		if isDebug {
			debugStr = "true"
		}
		// Only trace metrics have samplerType tag
		tags := map[string]string{"svc": serviceName, "debug": debugStr, samplerTypeKey: samplerType.String()}

		c := m.factory.Counter(metrics.Options{Name: m.category, Tags: tags})
		counts[key] = c
		counter = c
	} else {
		otherServicesSampler, ok := otherServicesSamplers[samplerType]
		if !ok {
			otherServicesSampler = otherServicesSamplers[model.SamplerTypeUnrecognized]
		}
		counter = counts[otherServicesSampler]
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
	serviceName = normalizer.ServiceName(serviceName)
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
