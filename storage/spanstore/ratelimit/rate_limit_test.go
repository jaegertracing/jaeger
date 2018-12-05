// Copyright (c) 2018 Uber Technologies, Inc.
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

package ratelimit

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

type mockWriter struct {
	expectedError error
}

func (m *mockWriter) WriteSpan(s *model.Span) error {
	return m.expectedError
}

type mockRateLimiter struct {
	calls int
}

func (m *mockRateLimiter) Take() time.Time {
	m.calls++
	return time.Time{}
}

func TestRateLimitedWriter(t *testing.T) {
	writer := &mockWriter{}
	decoratedWriter, err := NewRateLimitedWriter(writer, 10)
	require.NoError(t, err)
	var rateLimiter mockRateLimiter
	decoratedWriter.(*rateLimitedWriter).limiter = &rateLimiter
	require.NotEqual(t, writer, decoratedWriter)
	err = decoratedWriter.WriteSpan(&model.Span{})
	require.NoError(t, err)
	require.Equal(t, 1, rateLimiter.calls)
}

func TestRateLimitedWriterInvalidWritesPerSecond(t *testing.T) {
	writer := &mockWriter{}
	decoratedWriter, err := NewRateLimitedWriter(writer, 0)
	require.Error(t, err)
	require.Nil(t, decoratedWriter)
}

func TestRateLimitedWriterWithWriteError(t *testing.T) {
	var fakeError = errors.New("test")
	writer := &mockWriter{
		expectedError: fakeError,
	}
	decoratedWriter, err := NewRateLimitedWriter(writer, 5)
	require.NoError(t, err)
	err = decoratedWriter.WriteSpan(nil)
	require.Error(t, err)
	require.Equal(t, fakeError, err)
	writer.expectedError = nil
	err = decoratedWriter.WriteSpan(nil)
	require.NoError(t, err)
}
