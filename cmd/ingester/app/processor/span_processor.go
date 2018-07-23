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
	"io"

	"github.com/pkg/errors"

	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

//go:generate mockery -name=SpanProcessor

// SpanProcessor processes kafka spans
type SpanProcessor interface {
	Process(input Message) error
	io.Closer
}

type spanProcessor struct {
	unmarshaller kafka.Unmarshaller
	writer       spanstore.Writer
	io.Closer
}

// Message contains the fields of the kafka message that the span processor uses
type Message interface {
	Value() []byte
}

// NewSpanProcessor creates a new SpanProcessor
func NewSpanProcessor(writer spanstore.Writer, unmarshaller kafka.Unmarshaller) SpanProcessor {
	return &spanProcessor{
		unmarshaller: unmarshaller,
		writer:       writer,
	}
}

// Process unmarshals and writes a single kafka message
func (s spanProcessor) Process(message Message) error {
	mSpan, err := s.unmarshaller.Unmarshal(message.Value())
	if err != nil {
		return errors.Wrap(err, "cannot unmarshall byte array into span")
	}
	return s.writer.WriteSpan(mSpan)
}
