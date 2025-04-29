// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"context"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

type spanWriterMetrics struct {
	SpansWrittenSuccess metrics.Counter
	SpansWrittenFailure metrics.Counter
}

// SpanWriter writes spans to kafka. Implements spanstore.Writer
type SpanWriter struct {
	metrics    spanWriterMetrics
	producer   tracestore.Writer
	marshaller Marshaller
	topic      string
}

// NewSpanWriter initiates and returns a new kafka spanwriter
func NewSpanWriter(
	producer tracestore.Writer,
	marshaller Marshaller,
	topic string,
	factory metrics.Factory,
	logger *zap.Logger,
) *SpanWriter {
	writeMetrics := spanWriterMetrics{
		SpansWrittenSuccess: factory.Counter(metrics.Options{Name: "kafka_spans_written", Tags: map[string]string{"status": "success"}}),
		SpansWrittenFailure: factory.Counter(metrics.Options{Name: "kafka_spans_written", Tags: map[string]string{"status": "failure"}}),
	}

	return &SpanWriter{
		producer:   producer,
		marshaller: marshaller,
		topic:      topic,
		metrics:    writeMetrics,
	}
}

// WriteSpan writes the span to kafka.
func (w *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	batch := &model.Batch{
		Spans: []*model.Span{span},
	}
	traces := v1adapter.V1BatchesToTraces([]*model.Batch{batch})

	// Write using tracestore.Writer interface
	err := w.producer.WriteTraces(ctx, traces)
	if err != nil {
		w.metrics.SpansWrittenFailure.Inc(1)
		return err
	}
	return nil
}

// Close closes SpanWriter by closing producer
func (w *SpanWriter) Close() error {
	return nil
}
