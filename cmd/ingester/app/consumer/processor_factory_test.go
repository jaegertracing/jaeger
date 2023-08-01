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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func Test_NewFactory(t *testing.T) {
	params := ProcessorFactoryParams{}
	newFactory, err := NewProcessorFactory(params)
	require.NoError(t, err)
	require.NotNil(t, newFactory)
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

func (f *fakeMsg) Key() []byte {
	return []byte("1")
}

func (f *fakeMsg) Value() []byte {
	return nil
}

func (f *fakeMsg) Topic() string {
	return "fake_msg_test"
}

func (f *fakeMsg) Partition() int32 {
	return 0
}

func (f *fakeMsg) Offset() int64 {
	return 1
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

type fakeConsumerGroupSession struct {
	topic     string
	partition int32
	offset    int64
}

func (s *fakeConsumerGroupSession) MarkOffset(topic string, partition int32, offset int64, metadata string) {
	s.topic = topic
	s.partition = partition
	s.offset = offset
}

type fakeConsumerGroupClaim struct{}

func (c *fakeConsumerGroupClaim) Topic() string {
	return "fake_msg_test"
}

func (c *fakeConsumerGroupClaim) Partition() int32 {
	return 0
}

func Test_New(t *testing.T) {
	logger, logBuf := testutils.NewLogger()

	processor := &fakeProcessor{}
	processor.On("Close").Return(nil)

	factoryParams := ProcessorFactoryParams{
		Parallelism:   1,
		Topic:         "fake_msg_test",
		BaseProcessor: processor,
		Logger:        logger,
		Factory:       metrics.NullFactory,
	}

	newFactory, err := NewProcessorFactory(factoryParams)
	require.NoError(t, err)
	require.NotNil(t, newFactory)

	session := &fakeConsumerGroupSession{}
	claim := &fakeConsumerGroupClaim{}

	sp := newFactory.new(session, claim, 0)

	msg := &fakeMsg{}
	processor.On("Process", msg).Return(nil)
	err = sp.Process(msg)
	require.NoError(t, err)

	sp.Close()

	assert.Equal(t, session.topic, msg.Topic())
	assert.Equal(t, session.partition, msg.Partition())
	assert.Equal(t, session.offset, msg.Offset())

	t.Logf("processor all logs: %s", logBuf.String())
}
