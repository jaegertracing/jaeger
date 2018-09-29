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

package metrics

import (
	"context"
	"time"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// ReadMetricsDecorator wraps a spanstore.Reader and collects metrics around each read operation.
type ReadMetricsDecorator struct {
	spanReader           spanstore.Reader
	findTracesMetrics    *queryMetrics
	getTraceMetrics      *queryMetrics
	getServicesMetrics   *queryMetrics
	getOperationsMetrics *queryMetrics
}

type queryMetrics struct {
	Errors     metrics.Counter `metric:"calls" tags:"result=err"`
	Successes  metrics.Counter `metric:"calls" tags:"result=ok"`
	Responses  metrics.Timer   `metric:"responses"` //used as a histogram, not necessary for GetTrace
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
		getTraceMetrics:      buildQueryMetrics("get_trace", metricsFactory),
		getServicesMetrics:   buildQueryMetrics("get_services", metricsFactory),
		getOperationsMetrics: buildQueryMetrics("get_operations", metricsFactory),
	}
}

func buildQueryMetrics(namespace string, metricsFactory metrics.Factory) *queryMetrics {
	qMetrics := &queryMetrics{}
	scoped := metricsFactory.Namespace(namespace, nil)
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

// GetTrace implements spanstore.Reader#GetTrace
func (m *ReadMetricsDecorator) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetTrace(ctx, traceID)
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
func (m *ReadMetricsDecorator) GetOperations(ctx context.Context, service string) ([]string, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetOperations(ctx, service)
	m.getOperationsMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}
