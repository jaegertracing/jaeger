// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"context"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	jaegerBatches = "jaeger"
	zipkinBatches = "zipkin"
)

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

	// counters grouped by the type of data format (Jaeger or Zipkin).
	metrics map[string]batchMetrics
}

// WrapWithMetrics wraps Reporter and creates metrics for its invocations.
func WrapWithMetrics(reporter Reporter, mFactory metrics.Factory) *MetricsReporter {
	batchesMetrics := map[string]batchMetrics{}
	for _, s := range []string{zipkinBatches, jaegerBatches} {
		bm := batchMetrics{}
		metrics.MustInit(&bm,
			mFactory.Namespace(metrics.NSOptions{
				Name: "reporter",
				Tags: map[string]string{"format": s},
			}),
			nil)
		batchesMetrics[s] = bm
	}
	return &MetricsReporter{wrapped: reporter, metrics: batchesMetrics}
}

// EmitZipkinBatch emits batch to collector.
func (r *MetricsReporter) EmitZipkinBatch(ctx context.Context, spans []*zipkincore.Span) error {
	err := r.wrapped.EmitZipkinBatch(ctx, spans)
	updateMetrics(r.metrics[zipkinBatches], int64(len(spans)), err)
	return err
}

// EmitBatch emits batch to collector.
func (r *MetricsReporter) EmitBatch(ctx context.Context, batch *jaeger.Batch) error {
	size := int64(0)
	if batch != nil {
		size = int64(len(batch.GetSpans()))
	}
	err := r.wrapped.EmitBatch(ctx, batch)
	updateMetrics(r.metrics[jaegerBatches], size, err)
	return err
}

func updateMetrics(m batchMetrics, size int64, err error) {
	if err != nil {
		m.BatchesFailures.Inc(1)
		m.SpansFailures.Inc(size)
	} else {
		m.BatchSize.Update(size)
		m.BatchesSubmitted.Inc(1)
		m.SpansSubmitted.Inc(size)
	}
}
