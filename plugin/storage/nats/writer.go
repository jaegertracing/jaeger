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

package nats

import (
	"github.com/nats-io/nats.go"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"


)

type spanWriterMetrics struct {
	SpansWrittenSuccess metrics.Counter
	SpansWrittenFailure metrics.Counter
}

// SpanWriter writes spans to kafka. Implements spanstore.Writer
type SpanWriter struct {
	metrics    spanWriterMetrics
	producer   *nats.Conn
	marshaller kafka.Marshaller
	subject    string
}

// NewSpanWriter initiates and returns a new NATS spanwriter
func NewSpanWriter(
	producer *nats.Conn,
	marshaller kafka.Marshaller,
	subject string,
	factory metrics.Factory,
	logger *zap.Logger,
) *SpanWriter {
	writeMetrics := spanWriterMetrics{
		SpansWrittenSuccess: factory.Counter(metrics.Options{Name: "nats_spans_written", Tags: map[string]string{"status": "success"}}),
		SpansWrittenFailure: factory.Counter(metrics.Options{Name: "nats_spans_written", Tags: map[string]string{"status": "failure"}}),
	}

	return &SpanWriter{
		producer:   producer,
		marshaller: marshaller,
		subject:    subject,
		metrics:    writeMetrics,
	}
}

// WriteSpan writes the span to NATS.
func (w *SpanWriter) WriteSpan(span *model.Span) error {
	spanBytes, err := w.marshaller.Marshal(span)
	if err != nil {
		w.metrics.SpansWrittenFailure.Inc(1)
		return err
	}

	// The AsyncProducer accepts messages on a channel and produces them asynchronously
	// in the background as efficiently as possible
	traceID := span.TraceID.String()
	return w.producer.Publish(traceID, spanBytes)
}

// Close closes SpanWriter by closing producer
func (w *SpanWriter) Close() error {
	w.producer.Close()
	return nil
}
