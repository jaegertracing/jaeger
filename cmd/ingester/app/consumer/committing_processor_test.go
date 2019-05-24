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

package consumer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	kafka "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/mocks"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

type fakeOffsetMarker struct {
	capturedOffset int64
}

func (f *fakeOffsetMarker) MarkOffset(o int64) error {
	f.capturedOffset = o
	return nil
}

func TestNewCommittingProcessor(t *testing.T) {
	msgOffset := int64(123)
	offsetMarker := &fakeOffsetMarker{}
	spanProcessor := &mocks.SpanProcessor{}
	spanProcessor.On("Process", mock.Anything).Return(nil)
	committingProcessor := NewCommittingProcessor(spanProcessor, offsetMarker)

	msg := &kafka.Message{}
	msg.On("Offset").Return(msgOffset)

	assert.NoError(t, committingProcessor.Process(msg))

	spanProcessor.AssertExpectations(t)
	assert.Equal(t, msgOffset, offsetMarker.capturedOffset)
}

func TestNewCommittingProcessorError(t *testing.T) {
	offsetMarker := &fakeOffsetMarker{}
	spanProcessor := &mocks.SpanProcessor{}
	spanProcessor.On("Process", mock.Anything).Return(errors.New("boop"))
	committingProcessor := NewCommittingProcessor(spanProcessor, offsetMarker)
	msg := &kafka.Message{}

	assert.Error(t, committingProcessor.Process(msg))

	spanProcessor.AssertExpectations(t)
	assert.Equal(t, int64(0), offsetMarker.capturedOffset)
}

type fakeProcessorMessage struct{}

func (f fakeProcessorMessage) Value() []byte {
	return nil
}

func TestNewCommittingProcessorErrorNoKafkaMessage(t *testing.T) {
	committingProcessor := NewCommittingProcessor(&mocks.SpanProcessor{}, &fakeOffsetMarker{})

	assert.Error(t, committingProcessor.Process(fakeProcessorMessage{}))
}
