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

package kafka

import (
	"context"

	"github.com/Shopify/sarama"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

type spanWriterMetrics struct {
	SpansWrittenSuccess metrics.Counter
	SpansWrittenFailure metrics.Counter
}

// SpanWriter writes spans to kafka. Implements spanstore.Writer
type SpanWriter struct {
	metrics    spanWriterMetrics
	producer   sarama.AsyncProducer
	marshaller Marshaller
	topic      string
}

// NewSpanWriter initiates and returns a new kafka spanwriter
func NewSpanWriter(
	producer sarama.AsyncProducer,
	marshaller Marshaller,
	topic string,
	factory metrics.Factory,
	logger *zap.Logger,
) *SpanWriter {
	writeMetrics := spanWriterMetrics{
		SpansWrittenSuccess: factory.Counter(metrics.Options{Name: "kafka_spans_written", Tags: map[string]string{"status": "success"}}),
		SpansWrittenFailure: factory.Counter(metrics.Options{Name: "kafka_spans_written", Tags: map[string]string{"status": "failure"}}),
	}

	go func() {
		for range producer.Successes() {
			writeMetrics.SpansWrittenSuccess.Inc(1)
		}
	}()
	go func() {
		for e := range producer.Errors() {
			if e != nil && e.Err != nil {
				logger.Error(e.Err.Error())
			}
			writeMetrics.SpansWrittenFailure.Inc(1)
		}
	}()

	return &SpanWriter{
		producer:   producer,
		marshaller: marshaller,
		topic:      topic,
		metrics:    writeMetrics,
	}
}

// WriteSpan writes the span to kafka.
func (w *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	spanBytes, err := w.marshaller.Marshal(span)
	if err != nil {
		w.metrics.SpansWrittenFailure.Inc(1)
		return err
	}

	// The AsyncProducer accepts messages on a channel and produces them asynchronously
	// in the background as efficiently as possible
	w.producer.Input() <- &sarama.ProducerMessage{
		Topic: w.topic,
		Key:   sarama.StringEncoder(span.TraceID.String()),
		Value: sarama.ByteEncoder(spanBytes),
	}
	return nil
}

// Close closes SpanWriter by closing producer
func (w *SpanWriter) Close() error {
	return w.producer.Close()
}
