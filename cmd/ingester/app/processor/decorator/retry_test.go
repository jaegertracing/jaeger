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

package decorator

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

type fakeMsg struct{}

func (fakeMsg) Value() []byte {
	return nil
}
func TestNewRetryingProcessor(t *testing.T) {
	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(nil)
	lf := metrics.NewLocalFactory(0)
	rp := NewRetryingProcessor(lf, mockProcessor)

	assert.NoError(t, rp.Process(msg))

	mockProcessor.AssertExpectations(t)
	c, _ := lf.Snapshot()
	assert.Equal(t, int64(0), c["span-processor.retry-exhausted"])
	assert.Equal(t, int64(0), c["span-processor.retry-attempts"])
}

func TestNewRetryingProcessorError(t *testing.T) {
	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(errors.New("retry"))
	opts := []RetryOption{
		MinBackoffInterval(0),
		MaxBackoffInterval(time.Second),
		MaxAttempts(2),
		PropagateError(true),
		Rand(&fakeRand{})}
	lf := metrics.NewLocalFactory(0)
	rp := NewRetryingProcessor(lf, mockProcessor, opts...)

	assert.Error(t, rp.Process(msg))

	mockProcessor.AssertNumberOfCalls(t, "Process", 3)
	c, _ := lf.Snapshot()
	assert.Equal(t, int64(1), c["span-processor.retry-exhausted"])
	assert.Equal(t, int64(2), c["span-processor.retry-attempts"])
}

func TestNewRetryingProcessorNoErrorPropagation(t *testing.T) {
	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(errors.New("retry"))
	opts := []RetryOption{
		MinBackoffInterval(0),
		MaxBackoffInterval(time.Second),
		MaxAttempts(1),
		PropagateError(false),
		Rand(&fakeRand{})}

	lf := metrics.NewLocalFactory(0)
	rp := NewRetryingProcessor(lf, mockProcessor, opts...)

	assert.NoError(t, rp.Process(msg))
	mockProcessor.AssertNumberOfCalls(t, "Process", 2)
	c, _ := lf.Snapshot()
	assert.Equal(t, int64(1), c["span-processor.retry-exhausted"])
	assert.Equal(t, int64(1), c["span-processor.retry-attempts"])
}

type fakeRand struct{}

func (f *fakeRand) Int63n(v int64) int64 {
	return v
}

func Test_ProcessBackoff(t *testing.T) {
	minBackoff := time.Second
	maxBackoff := time.Minute
	tests := []struct {
		name             string
		attempt          uint
		expectedInterval time.Duration
	}{
		{
			name:             "zeroth retry attempt, minBackoff",
			attempt:          0,
			expectedInterval: minBackoff,
		},
		{
			name:             "first retry attempt, 2 x minBackoff",
			attempt:          1,
			expectedInterval: 2 * minBackoff,
		},
		{
			name:             "second attempt, 4 x minBackoff",
			attempt:          2,
			expectedInterval: 2 * 2 * minBackoff,
		},
		{
			name:             "sixth attempt, maxBackoff",
			attempt:          6,
			expectedInterval: maxBackoff,
		},
		{
			name:             "overflows, maxBackoff",
			attempt:          64,
			expectedInterval: maxBackoff,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rd := &retryDecorator{
				retryAttempts: metrics.NullCounter,
				options: retryOptions{
					minInterval: minBackoff,
					maxInterval: maxBackoff,
					rand:        &fakeRand{},
				},
			}
			assert.Equal(t, tt.expectedInterval, rd.computeInterval(tt.attempt))
		})
	}
}
