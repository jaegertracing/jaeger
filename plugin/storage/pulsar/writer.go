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

package pulsar

import (
	"context"

	"github.com/apache/pulsar-client-go/pulsar"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type spanWriterMetrics struct {
	SpansWrittenSuccess metrics.Counter
	SpansWrittenFailure metrics.Counter
}

// SpanWriter writes spans to pulsar. Implements spanstore.Writer
type SpanWriter struct {
	metrics    spanWriterMetrics
	producer   pulsar.Producer
	marshaller Marshaller
	// topic      string
}

// NewSpanWriter initiates and returns a new pulsar spanwriter
func NewSpanWriter(
	producer pulsar.Producer,
	marshaller Marshaller,
	factory metrics.Factory,
	logger *zap.Logger,
) *SpanWriter {
	writeMetrics := spanWriterMetrics{
		SpansWrittenSuccess: factory.Counter(metrics.Options{Name: "pulsar_spans_written", Tags: map[string]string{"status": "success"}}),
		SpansWrittenFailure: factory.Counter(metrics.Options{Name: "pulsar_spans_written", Tags: map[string]string{"status": "failure"}}),
	}

	return &SpanWriter{
		producer:   producer,
		marshaller: marshaller,
		// topic:      topic,
		metrics: writeMetrics,
	}
}

// WriteSpan writes the span to pulsar.
func (w *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	spanBytes, err := w.marshaller.Marshal(span)
	if err != nil {
		w.metrics.SpansWrittenFailure.Inc(1)
		return err
	}

	callback := func(id pulsar.MessageID, msg *pulsar.ProducerMessage, err error) {
		if err == nil {
			w.metrics.SpansWrittenSuccess.Inc(1)
			return
		}
		w.metrics.SpansWrittenFailure.Inc(1)
	}

	// The AsyncProducer accepts messages on a channel and produces them asynchronously
	// in the background as efficiently as possible
	w.producer.SendAsync(ctx, &pulsar.ProducerMessage{
		Key:     span.TraceID.String(),
		Payload: spanBytes,
	}, callback)

	return nil
}

// Close closes SpanWriter by closing producer
func (w *SpanWriter) Close() error {
	w.producer.Flush()
	w.producer.Close()
	return nil
}
