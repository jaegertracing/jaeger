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

package processor

import (
	"context"
	"fmt"
	"io"

	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
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
	writer       spanstore.Writer
	io.Closer
}

// NewSpanProcessor creates a new KafkaSpanProcessor
func NewSpanProcessor(params SpanProcessorParams) *KafkaSpanProcessor {
	return &KafkaSpanProcessor{
		unmarshaller: params.Unmarshaller,
		writer:       params.Writer,
	}
}

// Process unmarshals and writes a single kafka message
func (s KafkaSpanProcessor) Process(message Message) error {
	span, err := s.unmarshaller.Unmarshal(message.Value())
	if err != nil {
		return fmt.Errorf("cannot unmarshall byte array into span: %w", err)
	}
	// TODO context should be propagated from upstream components
	return s.writer.WriteSpan(context.TODO(), span)
}
