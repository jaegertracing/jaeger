// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	saramaMocks "github.com/Shopify/sarama/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka/mocks"
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

type spanWriterTest struct {
	producer       *saramaMocks.AsyncProducer
	marshaller     *mocks.Marshaller
	metricsFactory *metricstest.Factory

	writer *SpanWriter
}

// Checks that Kafka SpanWriter conforms to spanstore.Writer API
var _ spanstore.Writer = &SpanWriter{}

func withSpanWriter(t *testing.T, fn func(span *model.Span, w *spanWriterTest)) {
	serviceMetrics := metricstest.NewFactory(100 * time.Millisecond)
	defer serviceMetrics.Stop()
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.Return.Successes = true
	producer := saramaMocks.NewAsyncProducer(t, saramaConfig)
	marshaller := &mocks.Marshaller{}
	marshaller.On("Marshal", mock.AnythingOfType("*model.Span")).Return([]byte{}, nil)

	writerTest := &spanWriterTest{
		producer:       producer,
		marshaller:     marshaller,
		metricsFactory: serviceMetrics,
		writer:         NewSpanWriter(producer, marshaller, "someTopic", serviceMetrics, zap.NewNop()),
	}

	fn(sampleSpan, writerTest)
}

func TestKafkaWriter(t *testing.T) {
	withSpanWriter(t, func(span *model.Span, w *spanWriterTest) {
		w.producer.ExpectInputAndSucceed()

		err := w.writer.WriteSpan(context.Background(), span)
		require.NoError(t, err)

		for i := 0; i < 100; i++ {
			time.Sleep(time.Microsecond)
			counters, _ := w.metricsFactory.Snapshot()
			if counters["kafka_spans_written|status=success"] > 0 {
				break
			}
		}
		w.writer.Close()

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "kafka_spans_written",
				Tags:  map[string]string{"status": "success"},
				Value: 1,
			})

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "kafka_spans_written",
				Tags:  map[string]string{"status": "failure"},
				Value: 0,
			})
	})
}

func TestKafkaWriterErr(t *testing.T) {
	withSpanWriter(t, func(span *model.Span, w *spanWriterTest) {
		w.producer.ExpectInputAndFail(sarama.ErrRequestTimedOut)
		err := w.writer.WriteSpan(context.Background(), span)
		require.NoError(t, err)

		for i := 0; i < 100; i++ {
			time.Sleep(time.Microsecond)
			counters, _ := w.metricsFactory.Snapshot()
			if counters["kafka_spans_written|status=failure"] > 0 {
				break
			}
		}
		w.writer.Close()

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "kafka_spans_written",
				Tags:  map[string]string{"status": "success"},
				Value: 0,
			})

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "kafka_spans_written",
				Tags:  map[string]string{"status": "failure"},
				Value: 1,
			})
	})
}

func TestMarshallerErr(t *testing.T) {
	withSpanWriter(t, func(span *model.Span, w *spanWriterTest) {
		marshaller := &mocks.Marshaller{}
		marshaller.On("Marshal", mock.AnythingOfType("*model.Span")).Return([]byte{}, errors.New(""))
		w.writer.marshaller = marshaller

		err := w.writer.WriteSpan(context.Background(), span)
		require.Error(t, err)

		w.writer.Close()

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "kafka_spans_written",
				Tags:  map[string]string{"status": "success"},
				Value: 0,
			})

		w.metricsFactory.AssertCounterMetrics(t,
			metricstest.ExpectedMetric{
				Name:  "kafka_spans_written",
				Tags:  map[string]string{"status": "failure"},
				Value: 1,
			})
	})
}
