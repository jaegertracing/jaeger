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
	"github.com/stretchr/testify/mock"

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
	mockUnmarshaller := &umocks.Unmarshaller{}
	mockWriter := &smocks.Writer{}
	processor := NewSpanProcessor(SpanProcessorParams{
		Unmarshaller: mockUnmarshaller,
		Writer:       mockWriter,
	})

	message := &cmocks.Message{}
	data := []byte("irrelevant, mock unmarshaller should return the span")
	span := &model.Span{
		Process: nil, // we want to make sure sanitizers will fix this data issue.
	}

	message.On("Value").Return(data)
	mockUnmarshaller.On("Unmarshal", data).Return(span, nil)
	mockWriter.On("WriteSpan", context.Background(), span).
		Return(nil).
		Run(func(args mock.Arguments) {
			span := args[1].(*model.Span)
			assert.NotNil(t, span.Process, "sanitizer must fix Process=nil data issue")
		})

	assert.Nil(t, processor.Process(message))

	message.AssertExpectations(t)
	mockWriter.AssertExpectations(t)
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
	writer.AssertExpectations(t)
	writer.AssertNotCalled(t, "WriteSpan")
}
