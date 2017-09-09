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
	"time"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
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
	Errors     metrics.Counter `metric:"errors"`
	Attempts   metrics.Counter `metric:"attempts"`
	Successes  metrics.Counter `metric:"successes"`
	Responses  metrics.Timer   `metric:"responses"` //used as a histogram, not necessary for GetTrace
	ErrLatency metrics.Timer   `metric:"errLatency"`
	OKLatency  metrics.Timer   `metric:"okLatency"`
}

func (q *queryMetrics) emit(err error, latency time.Duration, responses int) {
	q.Attempts.Inc(1)
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
		findTracesMetrics:    buildQueryMetrics("FindTraces", metricsFactory),
		getTraceMetrics:      buildQueryMetrics("GetTrace", metricsFactory),
		getServicesMetrics:   buildQueryMetrics("GetServices", metricsFactory),
		getOperationsMetrics: buildQueryMetrics("GetOperations", metricsFactory),
	}
}

func buildQueryMetrics(namespace string, metricsFactory metrics.Factory) *queryMetrics {
	qMetrics := &queryMetrics{}
	scoped := metricsFactory.Namespace(namespace, nil)
	metrics.Init(qMetrics, scoped, nil)
	return qMetrics
}

// FindTraces implements spanstore.Reader#FindTraces
func (m *ReadMetricsDecorator) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	start := time.Now()
	retMe, err := m.spanReader.FindTraces(traceQuery)
	m.findTracesMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}

// GetTrace implements spanstore.Reader#GetTrace
func (m *ReadMetricsDecorator) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetTrace(traceID)
	m.getTraceMetrics.emit(err, time.Since(start), 1)
	return retMe, err
}

// GetServices implements spanstore.Reader#GetServices
func (m *ReadMetricsDecorator) GetServices() ([]string, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetServices()
	m.getServicesMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}

// GetOperations implements spanstore.Reader#GetOperations
func (m *ReadMetricsDecorator) GetOperations(service string) ([]string, error) {
	start := time.Now()
	retMe, err := m.spanReader.GetOperations(service)
	m.getOperationsMetrics.emit(err, time.Since(start), len(retMe))
	return retMe, err
}
