// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package decorator

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type fakeMsg struct{}

func (fakeMsg) Value() []byte {
	return nil
}

func TestNewRetryingProcessor(t *testing.T) {
	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(nil)
	lf := metricstest.NewFactory(0)
	rp := NewRetryingProcessor(lf, mockProcessor)

	require.NoError(t, rp.Process(msg))

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
		Rand(&fakeRand{}),
	}
	lf := metricstest.NewFactory(0)
	rp := NewRetryingProcessor(lf, mockProcessor, opts...)

	require.Error(t, rp.Process(msg))

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
		Rand(&fakeRand{}),
	}

	lf := metricstest.NewFactory(0)
	rp := NewRetryingProcessor(lf, mockProcessor, opts...)

	require.NoError(t, rp.Process(msg))
	mockProcessor.AssertNumberOfCalls(t, "Process", 2)
	c, _ := lf.Snapshot()
	assert.Equal(t, int64(1), c["span-processor.retry-exhausted"])
	assert.Equal(t, int64(1), c["span-processor.retry-attempts"])
}

type fakeRand struct{}

func (*fakeRand) Int63n(v int64) int64 {
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
		t.Run(tt.name, func(_ *testing.T) {
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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
