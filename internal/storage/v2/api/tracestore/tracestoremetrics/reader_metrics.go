// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestoremetrics

import (
	"context"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ tracestore.Reader = (*ReadMetricsDecorator)(nil)

// ReadMetricsDecorator wraps a tracestore.Reader and collects metrics around each read operation.
type ReadMetricsDecorator struct {
	traceReader          tracestore.Reader
	findTracesMetrics    *queryMetrics
	findTraceIDsMetrics  *queryMetrics
	getTraceMetrics      *queryMetrics
	getServicesMetrics   *queryMetrics
	getOperationsMetrics *queryMetrics
}

type queryMetrics struct {
	Errors     metrics.Counter `metric:"requests" tags:"result=err"`
	Successes  metrics.Counter `metric:"requests" tags:"result=ok"`
	Responses  metrics.Counter `metric:"responses"`
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
		q.Responses.Inc(int64(responses))
	}
}

// NewReaderDecorator returns a new ReadMetricsDecorator.
func NewReaderDecorator(traceReader tracestore.Reader, metricsFactory metrics.Factory) *ReadMetricsDecorator {
	return &ReadMetricsDecorator{
		traceReader:          traceReader,
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

// FindTraces implements tracestore.Reader#FindTraces
func (m *ReadMetricsDecorator) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		start := time.Now()
		var err error
		length := 0
		defer func() {
			m.findTracesMetrics.emit(err, time.Since(start), length)
		}()
		findTracesIter := m.traceReader.FindTraces(ctx, query)
		for traces, iterErr := range findTracesIter {
			err = iterErr
			length += len(traces)
			if !yield(traces, iterErr) {
				return
			}
		}
	}
}

// FindTraceIDs implements tracestore.Reader#FindTraceIDs
func (m *ReadMetricsDecorator) FindTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		start := time.Now()
		var err error
		length := 0
		defer func() {
			m.findTraceIDsMetrics.emit(err, time.Since(start), length)
		}()
		findTraceIDsIter := m.traceReader.FindTraceIDs(ctx, query)
		for traceIds, iterErr := range findTraceIDsIter {
			err = iterErr
			length += len(traceIds)
			if !yield(traceIds, iterErr) {
				return
			}
		}
	}
}

// GetTraces implements tracestore.Reader#GetTraces
func (m *ReadMetricsDecorator) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		start := time.Now()
		var err error
		length := 0
		defer func() {
			m.getTraceMetrics.emit(err, time.Since(start), length)
		}()
		getTraceIter := m.traceReader.GetTraces(ctx, traceIDs...)
		for traces, iterErr := range getTraceIter {
			err = iterErr
			length += len(traces)
			if !yield(traces, iterErr) {
				return
			}
		}
	}
}

// GetServices implements tracestore.Reader#GetServices
func (m *ReadMetricsDecorator) GetServices(ctx context.Context) ([]string, error) {
	start := time.Now()
	retMe, err := m.traceReader.GetServices(ctx)
	m.getServicesMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}

// GetOperations implements tracestore.Reader#GetOperations
func (m *ReadMetricsDecorator) GetOperations(
	ctx context.Context,
	query tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	start := time.Now()
	retMe, err := m.traceReader.GetOperations(ctx, query)
	m.getOperationsMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}
