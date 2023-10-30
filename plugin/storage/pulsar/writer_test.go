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
	"errors"
	"testing"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	sampleTags = model.KeyValues{
		model.String("someStringTagKey", "someStringTagValue"),
	}
	sampleSpan = &model.Span{
		TraceID:       model.TraceID{High: 22222, Low: 44444},
		SpanID:        model.SpanID(3333),
		OperationName: "someOperationName",
		References: []model.SpanRef{
			{
				TraceID: model.TraceID{High: 22222, Low: 44444},
				SpanID:  model.SpanID(11111),
				RefType: model.ChildOf,
			},
		},
		Flags:     model.Flags(1),
		StartTime: model.EpochMicrosecondsAsTime(55555),
		Duration:  model.MicrosecondsAsDuration(50000),
		Tags:      sampleTags,
		Logs: []model.Log{
			{
				Timestamp: model.EpochMicrosecondsAsTime(12345),
				Fields:    sampleTags,
			},
		},
		Process: &model.Process{
			ServiceName: "someServiceName",
			Tags:        sampleTags,
		},
	}
)

type mockProducer struct {
	expectError error
}

func newAsyncProducer() pulsar.Producer {
	return &mockProducer{}
}

func (mp *mockProducer) Topic() string { return "" }

func (mp *mockProducer) SetError(err error) { mp.expectError = err }

func (mp *mockProducer) Name() string { return "" }

func (mp *mockProducer) Send(_ context.Context, _ *pulsar.ProducerMessage) (pulsar.MessageID, error) {
	if mp.expectError != nil {
		return nil, mp.expectError
	}
	return nil, nil
}

func (mp *mockProducer) SendAsync(_ context.Context, _ *pulsar.ProducerMessage, cb func(_ pulsar.MessageID, _ *pulsar.ProducerMessage, err error)) {
	if mp.expectError != nil {
		cb(nil, nil, mp.expectError)
		return
	}

	cb(nil, nil, nil)
}

func (mp *mockProducer) LastSequenceID() int64 { return 0 }

func (mp *mockProducer) Flush() error {
	if mp.expectError != nil {
		return mp.expectError
	}
	return nil
}

func (mp *mockProducer) Close() {}

type spanWriterTest struct {
	producer       pulsar.Producer
	metricsFactory *metricstest.Factory

	writer *SpanWriter
}

// Checks that pulsar SpanWriter conforms to spanstore.Writer API
var _ spanstore.Writer = &SpanWriter{}

func withSpanWriter(t *testing.T, fn func(span *model.Span, w *spanWriterTest)) {
	serviceMetrics := metricstest.NewFactory(100 * time.Millisecond)

	producer := newAsyncProducer()
	marshaller := newJSONMarshaller()

	writerTest := &spanWriterTest{
		producer:       producer,
		metricsFactory: serviceMetrics,
		writer:         NewSpanWriter(producer, marshaller, serviceMetrics, zap.NewNop()),
	}

	fn(sampleSpan, writerTest)
	producer.Close()
}

func TestPulsarWriter(t *testing.T) {
	withSpanWriter(t, func(span *model.Span, w *spanWriterTest) {
		err := w.writer.WriteSpan(context.Background(), span)
		assert.NoError(t, err)

		for i := 0; i < 100; i++ {
			time.Sleep(time.Microsecond)
			counters, _ := w.metricsFactory.Snapshot()
			if counters["pulsar_spans_written|status=success"] > 0 {
				break
			}
		}
		w.writer.Close()

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "pulsar_spans_written",
				Tags:  map[string]string{"status": "success"},
				Value: 1,
			})

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "pulsar_spans_written",
				Tags:  map[string]string{"status": "failure"},
				Value: 0,
			})
	})
}

func TestPulsarWriterErr(t *testing.T) {
	withSpanWriter(t, func(span *model.Span, w *spanWriterTest) {
		// set expect error
		w.producer.(*mockProducer).SetError(errors.New("raise error"))

		err := w.writer.WriteSpan(context.Background(), span)
		assert.NoError(t, err)

		for i := 0; i < 100; i++ {
			time.Sleep(time.Microsecond)
			counters, _ := w.metricsFactory.Snapshot()
			if counters["pulsar_spans_written|status=failure"] > 0 {
				break
			}
		}

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "pulsar_spans_written",
				Tags:  map[string]string{"status": "success"},
				Value: 0,
			})

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "pulsar_spans_written",
				Tags:  map[string]string{"status": "failure"},
				Value: 1,
			})
	})
}
