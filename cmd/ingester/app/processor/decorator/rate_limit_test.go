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

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

func TestNewRateLimitingProcessor(t *testing.T) {
	const (
		creditsPerSecond = 1000.0
		maxBalance       = 1.0
	)

	mockProcessor := &mocks.SpanProcessor{}
	msg := &fakeMsg{}
	mockProcessor.On("Process", msg).Return(nil)
	rp := NewRateLimitingProcessor(mockProcessor, CreditsPerSecond(creditsPerSecond), MaxBalance(maxBalance))
	start := time.Now()
	assert.NoError(t, rp.Process(msg))
	firstCallDuration := time.Since(start)
	start = time.Now()
	assert.NoError(t, rp.Process(msg))
	secondCallDuration := time.Since(start)
	assert.Truef(t, firstCallDuration < secondCallDuration, "first call was slower than second call, first call duration = %v, second call duration = %v", firstCallDuration, secondCallDuration)
	mockProcessor.AssertExpectations(t)
}

func TestNewRateLimitingProcessorError(t *testing.T) {
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
