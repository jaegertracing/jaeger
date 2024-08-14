// Copyright (c) 2022 The Jaeger Authors.
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

package metrics

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	protometrics "github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

// ReadMetricsDecorator wraps a metricsstore.Reader and collects metrics around each read operation.
type ReadMetricsDecorator struct {
	reader                    metricsstore.Reader
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
func NewReadMetricsDecorator(reader metricsstore.Reader, metricsFactory metrics.Factory) *ReadMetricsDecorator {
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

// GetLatencies implements metricsstore.Reader#GetLatencies
func (m *ReadMetricsDecorator) GetLatencies(ctx context.Context, params *metricsstore.LatenciesQueryParameters) (*protometrics.MetricFamily, error) {
	start := time.Now()
	retMe, err := m.reader.GetLatencies(ctx, params)
	m.getLatenciesMetrics.emit(err, time.Since(start))
	return retMe, err
}

// GetCallRates implements metricsstore.Reader#GetCallRates
func (m *ReadMetricsDecorator) GetCallRates(ctx context.Context, params *metricsstore.CallRateQueryParameters) (*protometrics.MetricFamily, error) {
	start := time.Now()
	retMe, err := m.reader.GetCallRates(ctx, params)
	m.getCallRatesMetrics.emit(err, time.Since(start))
	return retMe, err
}

// GetErrorRates implements metricsstore.Reader#GetErrorRates
func (m *ReadMetricsDecorator) GetErrorRates(ctx context.Context, params *metricsstore.ErrorRateQueryParameters) (*protometrics.MetricFamily, error) {
	start := time.Now()
	retMe, err := m.reader.GetErrorRates(ctx, params)
	m.getErrorRatesMetrics.emit(err, time.Since(start))
	return retMe, err
}

// GetMinStepDuration implements metricsstore.Reader#GetMinStepDuration
func (m *ReadMetricsDecorator) GetMinStepDuration(ctx context.Context, params *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	start := time.Now()
	retMe, err := m.reader.GetMinStepDuration(ctx, params)
	m.getMinStepDurationMetrics.emit(err, time.Since(start))
	return retMe, err
}
