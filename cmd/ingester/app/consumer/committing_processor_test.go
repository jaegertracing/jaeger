// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	kafka "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/mocks"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

type fakeOffsetMarker struct {
	capturedOffset int64
}

func (f *fakeOffsetMarker) MarkOffset(o int64) {
	f.capturedOffset = o
}

func TestNewCommittingProcessor(t *testing.T) {
	msgOffset := int64(123)
	offsetMarker := &fakeOffsetMarker{}
	spanProcessor := &mocks.SpanProcessor{}
	spanProcessor.On("Process", mock.Anything).Return(nil)
	committingProcessor := NewCommittingProcessor(spanProcessor, offsetMarker)

	msg := &kafka.Message{}
	msg.On("Offset").Return(msgOffset)

	require.NoError(t, committingProcessor.Process(msg))

	spanProcessor.AssertExpectations(t)
	assert.Equal(t, msgOffset, offsetMarker.capturedOffset)
}

func TestNewCommittingProcessorError(t *testing.T) {
	offsetMarker := &fakeOffsetMarker{}
	spanProcessor := &mocks.SpanProcessor{}
	spanProcessor.On("Process", mock.Anything).Return(errors.New("boop"))
	committingProcessor := NewCommittingProcessor(spanProcessor, offsetMarker)
	msg := &kafka.Message{}

	require.Error(t, committingProcessor.Process(msg))

	spanProcessor.AssertExpectations(t)
	assert.Equal(t, int64(0), offsetMarker.capturedOffset)
}

type fakeProcessorMessage struct{}

func (fakeProcessorMessage) Value() []byte {
	return nil
}

func TestNewCommittingProcessorErrorNoKafkaMessage(t *testing.T) {
	committingProcessor := NewCommittingProcessor(&mocks.SpanProcessor{}, &fakeOffsetMarker{})

	require.Error(t, committingProcessor.Process(fakeProcessorMessage{}))
}
