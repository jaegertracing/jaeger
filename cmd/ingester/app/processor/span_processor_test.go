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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	cmocks "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/mocks"
	"github.com/jaegertracing/jaeger/model"
	umocks "github.com/jaegertracing/jaeger/pkg/kafka/mocks"
	smocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestNewSpanProcessor(t *testing.T) {
	p := SpanProcessorParams{}
	assert.NotNil(t, NewSpanProcessor(p))
}

func TestSpanProcessor_Process(t *testing.T) {
	writer := &smocks.Writer{}
	unmarshallerMock := &umocks.Unmarshaller{}
	processor := &KafkaSpanProcessor{
		unmarshaller: unmarshallerMock,
		writer:       writer,
	}

	message := &cmocks.Message{}
	data := []byte("police")
	span := &model.Span{}

	message.On("Value").Return(data)
	unmarshallerMock.On("Unmarshal", data).Return(span, nil)
	writer.On("WriteSpan", context.Background(), span).Return(nil)

	assert.Nil(t, processor.Process(message))

	message.AssertExpectations(t)
	writer.AssertExpectations(t)
}

func TestSpanProcessor_ProcessError(t *testing.T) {
	writer := &smocks.Writer{}
	unmarshallerMock := &umocks.Unmarshaller{}
	processor := &KafkaSpanProcessor{
		unmarshaller: unmarshallerMock,
		writer:       writer,
	}

	message := &cmocks.Message{}
	data := []byte("police")

	message.On("Value").Return(data)
	unmarshallerMock.On("Unmarshal", data).Return(nil, errors.New("moocow"))

	assert.Error(t, processor.Process(message))

	message.AssertExpectations(t)
	writer.AssertNotCalled(t, "WriteSpan")
}
