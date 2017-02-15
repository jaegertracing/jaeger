// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
