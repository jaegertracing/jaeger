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

package processor_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	mockProcessor "github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

type fakeMessage struct{}

func (fakeMessage) Value() []byte {
	return nil
}

func TestNewParallelProcessor(t *testing.T) {
	msg := &fakeMessage{}
	mp := &mockProcessor.SpanProcessor{}
	mp.On("Process", msg).Return(nil)

	pp := processor.NewParallelProcessor(mp, 1, zap.NewNop())
	pp.Start()

	pp.Process(msg, nil)
	pp.Close()

	mp.AssertExpectations(t)
}

func TestParallelProcessorError(t *testing.T) {
	msg := &fakeMessage{}
	mp := &mockProcessor.SpanProcessor{}
	err := errors.New("test error")
	mp.On("Process", msg).Return(err)

	pp := processor.NewParallelProcessor(mp, 1, zap.NewNop())
	pp.Start()
	defer pp.Close()

	wg := sync.WaitGroup{}
	wg.Add(1)

	pp.Process(msg, func(m processor.Message, e error) {
		assert.Equal(t, msg, m)
		assert.Equal(t, err, e)
		wg.Done()
	})

	wg.Wait()
	mp.AssertExpectations(t)
}
