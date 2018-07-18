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
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

func Test_new(t *testing.T) {

	mockConsumer := &mockSaramaConsumer{}
	mockConsumer.On("MarkPartitionOffset", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	pf := processorFactory{
		topic:          "testing",
		consumer:       mockConsumer,
		metricsFactory: metrics.NullFactory,
		logger:         zap.NewNop(),
		baseProcessor:  &mocks.SpanProcessor{},
		parallelism:    1,
	}

	assert.NotNil(t, pf.new(1, 2))
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

func (f *fakeMsg) Value() []byte {
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
