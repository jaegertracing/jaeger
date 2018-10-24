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

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

func TestNewRateLimitingProcessor(t *testing.T) {
	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(nil)
	rp := NewRateLimitingProcessor(mockProcessor)

	assert.NoError(t, rp.Process(msg))

	mockProcessor.AssertExpectations(t)
}

func TestNewRateLimitingProcessorError(t *testing.T) {
	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(errors.New("retry"))
	opts := []RateLimitOption{
		CreditsPerSecond(1),
		MaxBalance(1),
	}
	rp := NewRateLimitingProcessor(mockProcessor, opts...)

	assert.Error(t, rp.Process(msg))

	mockProcessor.AssertNumberOfCalls(t, "Process", 1)
}

func TestNewRateLimitingProcessorNoErrorPropagation(t *testing.T) {
	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(errors.New("rateLimit"))
	opts := []RateLimitOption{
		CreditsPerSecond(1),
		MaxBalance(1),
	}

	rp := NewRateLimitingProcessor(mockProcessor, opts...)

	assert.Error(t, rp.Process(msg))
	mockProcessor.AssertNumberOfCalls(t, "Process", 1)
}
