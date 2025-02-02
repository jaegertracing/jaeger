// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor

import (
	"context"
	"fmt"
	"io"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer"
	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

//go:generate mockery -name=KafkaSpanProcessor

// SpanProcessor processes kafka spans
type SpanProcessor interface {
	Process(input Message) error
	io.Closer
}

// Message contains the fields of the kafka message that the span processor uses
type Message interface {
	Value() []byte
}

// SpanProcessorParams stores the necessary parameters for a SpanProcessor
type SpanProcessorParams struct {
	Writer       spanstore.Writer
	Unmarshaller kafka.Unmarshaller
}

// KafkaSpanProcessor implements SpanProcessor for Kafka messages
type KafkaSpanProcessor struct {
	unmarshaller kafka.Unmarshaller
	sanitizer    sanitizer.SanitizeSpan
	writer       spanstore.Writer
	io.Closer
}

// NewSpanProcessor creates a new KafkaSpanProcessor
func NewSpanProcessor(params SpanProcessorParams) *KafkaSpanProcessor {
	return &KafkaSpanProcessor{
		unmarshaller: params.Unmarshaller,
		writer:       params.Writer,
		sanitizer:    sanitizer.NewChainedSanitizer(sanitizer.NewStandardSanitizers()...),
	}
}

// Process unmarshals and writes a single kafka message
func (s KafkaSpanProcessor) Process(message Message) error {
	span, err := s.unmarshaller.Unmarshal(message.Value())
	if err != nil {
		return fmt.Errorf("cannot unmarshall byte array into span: %w", err)
	}

	// TODO context should be propagated from upstream components
	return s.writer.WriteSpan(context.TODO(), s.sanitizer(span))
}
