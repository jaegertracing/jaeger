// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// ReadMetricsDecorator wraps a spanstore.Reader and collects metrics around each read operation.
type ReadMetricsDecorator struct {
	spanReader           spanstore.Reader
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

// NewReadMetricsDecorator returns a new ReadMetricsDecorator.
func NewReadMetricsDecorator(spanReader spanstore.Reader, metricsFactory metrics.Factory) *ReadMetricsDecorator {
	return &ReadMetricsDecorator{
		spanReader:           spanReader,
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

// FindTraces implements spanstore.Reader#FindTraces
func (m *ReadMetricsDecorator) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	start := time.Now()
	retMe, err := m.spanReader.FindTraces(ctx, traceQuery)
	m.findTracesMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}

// FindTraceIDs implements spanstore.Reader#FindTraceIDs
func (m *ReadMetricsDecorator) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	start := time.Now()
	retMe, err := m.spanReader.FindTraceIDs(ctx, traceQuery)
	m.findTraceIDsMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}

// GetTrace implements spanstore.Reader#GetTrace
func (m *ReadMetricsDecorator) GetTrace(ctx context.Context, traceGet spanstore.TraceGetParameters) (*model.Trace, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetTrace(ctx, traceGet)
	m.getTraceMetrics.emit(err, time.Since(start), 1)
	return retMe, err
}

// GetServices implements spanstore.Reader#GetServices
func (m *ReadMetricsDecorator) GetServices(ctx context.Context) ([]string, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetServices(ctx)
	m.getServicesMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}

// GetOperations implements spanstore.Reader#GetOperations
func (m *ReadMetricsDecorator) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetOperations(ctx, query)
	m.getOperationsMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}
