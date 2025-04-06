// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type KafkaSpanWriter struct {
	producer *kafka.Producer
	topic    string
	logger   *zap.Logger
}

func NewKafkaSpanWriter(brokers []string, topic string, logger *zap.Logger) (*KafkaSpanWriter, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": brokers[0],
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &KafkaSpanWriter{
		producer: p,
		topic:    topic,
		logger:   logger,
	}, nil
}

func (w *KafkaSpanWriter) WriteSpans(ctx context.Context, td ptrace.Traces) error {
	marshaler := ptrace.ProtoMarshaler{}
	protoBufBytes, err := marshaler.MarshalTraces(td)
	if err != nil {
		return fmt.Errorf("failed to marshal trace data to protobuf: %w", err)
	}

	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &w.topic, Partition: kafka.PartitionAny},
		Value:          protoBufBytes,
	}

	if err := w.producer.Produce(msg, nil); err != nil {
		return fmt.Errorf("failed to send span to kafka: %w", err)
	}
	w.logger.Info("Span written to Kafka", zap.Int("bytes", len(protoBufBytes)))
	return nil
}

func (w *KafkaSpanWriter) Close() error {
	w.producer.Flush(1000)
	w.producer.Close()
	return nil
}
