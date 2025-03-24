// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	cmocks "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/mocks"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
	kmocks "github.com/jaegertracing/jaeger/internal/storage/kafka/consumer/mocks"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func Test_NewFactory(t *testing.T) {
	params := ProcessorFactoryParams{}
	newFactory, err := NewProcessorFactory(params)
	require.NoError(t, err)
	assert.NotNil(t, newFactory)
}

func Test_new(t *testing.T) {
	mockConsumer := &kmocks.Consumer{}
	mockConsumer.On("MarkPartitionOffset", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	topic := "coelacanth"
	partition := int32(21)
	offset := int64(555)

	sp := &mocks.SpanProcessor{}
	sp.On("Process", mock.Anything).Return(nil)

	pf := ProcessorFactory{
		consumer:       mockConsumer,
		metricsFactory: metrics.NullFactory,
		logger:         zap.NewNop(),
		baseProcessor:  sp,
		parallelism:    1,
	}

	processor := pf.new(topic, partition, offset)
	defer processor.Close()
	msg := &cmocks.Message{}
	msg.On("Offset").Return(offset + 1)
	processor.Process(msg)

	// This sleep is greater than offset manager's resetInterval to allow it a chance to
	// call MarkPartitionOffset.
	time.Sleep(150 * time.Millisecond)
	mockConsumer.AssertCalled(t, "MarkPartitionOffset", topic, partition, offset+1, "")
}

type fakeService struct {
	startCalled bool
	closeCalled bool
}

func (f *fakeService) Start() {
	f.startCalled = true
}

func (f *fakeService) Close() error {
	f.closeCalled = true
	return nil
}

type fakeProcessor struct {
	startCalled bool
	mocks.SpanProcessor
}

func (f *fakeProcessor) Start() {
	f.startCalled = true
}

type fakeMsg struct{}

func (*fakeMsg) Value() []byte {
	return nil
}

func Test_startedProcessor_Process(t *testing.T) {
	service := &fakeService{}
	processor := &fakeProcessor{}
	processor.On("Close").Return(nil)

	s := newStartedProcessor(processor, service)

	assert.True(t, service.startCalled)
	assert.True(t, processor.startCalled)

	msg := &fakeMsg{}
	processor.On("Process", msg).Return(nil)

	s.Process(msg)

	s.Close()
	assert.True(t, service.closeCalled)
	processor.AssertExpectations(t)
}
