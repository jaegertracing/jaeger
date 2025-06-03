// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestoremetrics

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

// ReadMetricsDecorator wraps a spanstore.Reader and collects metrics around each read operation.
type ReadMetricsDecorator struct {
	findTracesMetrics    *queryMetrics
	findTraceIDsMetrics  *queryMetrics
	getTraceMetrics      *queryMetrics
	getServicesMetrics   *queryMetrics
	getOperationsMetrics *queryMetrics
}

type queryMetrics struct {
	Errors     metrics.Counter `metric:"requests" tags:"result=err"`
	Successes  metrics.Counter `metric:"requests" tags:"result=ok"`
	Responses  metrics.Timer   `metric:"responses"` // used as a histogram, not necessary for GetTrace
	ErrLatency metrics.Timer   `metric:"latency" tags:"result=err"`
	OKLatency  metrics.Timer   `metric:"latency" tags:"result=ok"`
}

func (q *queryMetrics) emit(err error, latency time.Duration, responses int) {
	if err != nil {
		q.Errors.Inc(1)
		q.ErrLatency.Record(latency)
	} else {
		q.Successes.Inc(1)
		q.OKLatency.Record(latency)
		q.Responses.Record(time.Duration(responses))
	}
}

func (q *queryMetrics) emitMetrics(length int, err error) {
	start := time.Now()
	q.emit(err, time.Since(start), length)
}

// NewReaderDecorator returns a new ReadMetricsDecorator.
func NewReaderDecorator(metricsFactory metrics.Factory) *ReadMetricsDecorator {
	return &ReadMetricsDecorator{
		findTracesMetrics:    buildQueryMetrics("find_traces", metricsFactory),
		findTraceIDsMetrics:  buildQueryMetrics("find_trace_ids", metricsFactory),
		getTraceMetrics:      buildQueryMetrics("get_trace", metricsFactory),
		getServicesMetrics:   buildQueryMetrics("get_services", metricsFactory),
		getOperationsMetrics: buildQueryMetrics("get_operations", metricsFactory),
	}
}

func buildQueryMetrics(operation string, metricsFactory metrics.Factory) *queryMetrics {
	qMetrics := &queryMetrics{}
	scoped := metricsFactory.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"operation": operation}})
	metrics.Init(qMetrics, scoped, nil)
	return qMetrics
}

// MetricsFindTraces implements metrics emission for spanstore.Reader#FindTraces
func (m *ReadMetricsDecorator) MetricsFindTraces(tracesLength int, err error) {
	m.findTracesMetrics.emitMetrics(tracesLength, err)
}

// MetricsFindTraceID implements metrics emission for spanstore.Reader#FindTraceIDs
func (m *ReadMetricsDecorator) MetricsFindTraceID(traceIDsLength int, err error) {
	m.findTraceIDsMetrics.emitMetrics(traceIDsLength, err)
}

// MetricsGetTraces implements metrics emission for spanstore.Reader#GetTrace
func (m *ReadMetricsDecorator) MetricsGetTraces(tracesLength int, err error) {
	m.getTraceMetrics.emitMetrics(tracesLength, err)
}

// MetricsGetServices implements metrics emission for spanstore.Reader#GetServices
func (m *ReadMetricsDecorator) MetricsGetServices(servicesLength int, err error) {
	m.getServicesMetrics.emitMetrics(servicesLength, err)
}

// MetricsGetOperations implements metrics emission for spanstore.Reader#GetOperations
func (m *ReadMetricsDecorator) MetricsGetOperations(operationsLength int, err error) {
	m.getOperationsMetrics.emitMetrics(operationsLength, err)
}
