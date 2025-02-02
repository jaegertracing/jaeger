// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	cmocks "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/mocks"
	umocks "github.com/jaegertracing/jaeger/internal/storage/v1/kafka/mocks"
	smocks "github.com/jaegertracing/jaeger/internal/storage/v1/spanstore/mocks"
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
	mockWriter.On("WriteSpan", context.TODO(), span).
		Return(nil).
		Run(func(args mock.Arguments) {
			span := args[1].(*model.Span)
			assert.NotNil(t, span.Process, "sanitizer must fix Process=nil data issue")
		})

	require.NoError(t, processor.Process(message))

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

	require.Error(t, processor.Process(message))

	message.AssertExpectations(t)
	writer.AssertExpectations(t)
	writer.AssertNotCalled(t, "WriteSpan")
}
