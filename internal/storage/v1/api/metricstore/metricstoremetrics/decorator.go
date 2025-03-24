// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstoremetrics

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	protometrics "github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
)

// ReadMetricsDecorator wraps a metricstore.Reader and collects metrics around each read operation.
type ReadMetricsDecorator struct {
	reader                    metricstore.Reader
	getLatenciesMetrics       *queryMetrics
	getCallRatesMetrics       *queryMetrics
	getErrorRatesMetrics      *queryMetrics
	getMinStepDurationMetrics *queryMetrics
}

type queryMetrics struct {
	Errors     metrics.Counter `metric:"requests" tags:"result=err"`
	Successes  metrics.Counter `metric:"requests" tags:"result=ok"`
	ErrLatency metrics.Timer   `metric:"latency" tags:"result=err"`
	OKLatency  metrics.Timer   `metric:"latency" tags:"result=ok"`
}

func (q *queryMetrics) emit(err error, latency time.Duration) {
	if err != nil {
		q.Errors.Inc(1)
		q.ErrLatency.Record(latency)
	} else {
		q.Successes.Inc(1)
		q.OKLatency.Record(latency)
	}
}

// NewReadMetricsDecorator returns a new ReadMetricsDecorator.
func NewReaderDecorator(reader metricstore.Reader, metricsFactory metrics.Factory) *ReadMetricsDecorator {
	return &ReadMetricsDecorator{
		reader:                    reader,
		getLatenciesMetrics:       buildQueryMetrics("get_latencies", metricsFactory),
		getCallRatesMetrics:       buildQueryMetrics("get_call_rates", metricsFactory),
		getErrorRatesMetrics:      buildQueryMetrics("get_error_rates", metricsFactory),
		getMinStepDurationMetrics: buildQueryMetrics("get_min_step_duration", metricsFactory),
	}
}

func buildQueryMetrics(operation string, metricsFactory metrics.Factory) *queryMetrics {
	qMetrics := &queryMetrics{}
	scoped := metricsFactory.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"operation": operation}})
	metrics.Init(qMetrics, scoped, nil)
	return qMetrics
}

// GetLatencies implements metricstore.Reader#GetLatencies
func (m *ReadMetricsDecorator) GetLatencies(ctx context.Context, params *metricstore.LatenciesQueryParameters) (*protometrics.MetricFamily, error) {
	start := time.Now()
	retMe, err := m.reader.GetLatencies(ctx, params)
	m.getLatenciesMetrics.emit(err, time.Since(start))
	return retMe, err
}

// GetCallRates implements metricstore.Reader#GetCallRates
func (m *ReadMetricsDecorator) GetCallRates(ctx context.Context, params *metricstore.CallRateQueryParameters) (*protometrics.MetricFamily, error) {
	start := time.Now()
	retMe, err := m.reader.GetCallRates(ctx, params)
	m.getCallRatesMetrics.emit(err, time.Since(start))
	return retMe, err
}

// GetErrorRates implements metricstore.Reader#GetErrorRates
func (m *ReadMetricsDecorator) GetErrorRates(ctx context.Context, params *metricstore.ErrorRateQueryParameters) (*protometrics.MetricFamily, error) {
	start := time.Now()
	retMe, err := m.reader.GetErrorRates(ctx, params)
	m.getErrorRatesMetrics.emit(err, time.Since(start))
	return retMe, err
}

// GetMinStepDuration implements metricstore.Reader#GetMinStepDuration
func (m *ReadMetricsDecorator) GetMinStepDuration(ctx context.Context, params *metricstore.MinStepDurationQueryParameters) (time.Duration, error) {
	start := time.Now()
	retMe, err := m.reader.GetMinStepDuration(ctx, params)
	m.getMinStepDurationMetrics.emit(err, time.Since(start))
	return retMe, err
}
