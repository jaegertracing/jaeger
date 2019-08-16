// Copyright (c) 2018 The Jaeger Authors.
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

package reporter

import (
	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	jaegerBatches = "jaeger"
	zipkinBatches = "zipkin"
)

// ReporterMetrics holds metrics related to reporter
type batchMetrics struct {
	// Number of successful batch submissions to collector
	BatchesSubmitted metrics.Counter `metric:"batches.submitted"`

	// Number of failed batch submissions to collector
	BatchesFailures metrics.Counter `metric:"batches.failures"`

	// Number of spans in a batch submitted to collector
	BatchSize metrics.Gauge `metric:"batch_size"`

	// Number of successful span submissions to collector
	SpansSubmitted metrics.Counter `metric:"spans.submitted"`

	// Number of failed span submissions to collector
	SpansFailures metrics.Counter `metric:"spans.failures"`
}

// MetricsReporter is reporter with metrics integration.
type MetricsReporter struct {
	wrapped Reporter
	metrics map[string]batchMetrics
}

// WrapWithMetrics wraps Reporter and creates metrics for its invocations.
func WrapWithMetrics(reporter Reporter, mFactory metrics.Factory) *MetricsReporter {
	batchesMetrics := map[string]batchMetrics{}
	for _, s := range []string{zipkinBatches, jaegerBatches} {
		bm := batchMetrics{}
		metrics.Init(&bm, mFactory.Namespace(metrics.NSOptions{Name: "reporter", Tags: map[string]string{"format": s}}), nil)
		batchesMetrics[s] = bm
	}
	return &MetricsReporter{wrapped: reporter, metrics: batchesMetrics}
}

// EmitZipkinBatch emits batch to collector.
func (r *MetricsReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	err := r.wrapped.EmitZipkinBatch(spans)
	withMetrics(r.metrics[zipkinBatches], int64(len(spans)), err)
	return err
}

// EmitBatch emits batch to collector.
func (r *MetricsReporter) EmitBatch(batch *jaeger.Batch) error {
	size := int64(0)
	if batch != nil {
		size = int64(len(batch.GetSpans()))
	}
	err := r.wrapped.EmitBatch(batch)
	withMetrics(r.metrics[jaegerBatches], size, err)
	return err
}

func withMetrics(m batchMetrics, size int64, err error) {
	if err != nil {
		m.BatchesFailures.Inc(1)
		m.SpansFailures.Inc(size)
	} else {
		m.BatchSize.Update(size)
		m.BatchesSubmitted.Inc(1)
		m.SpansSubmitted.Inc(size)
	}
}
