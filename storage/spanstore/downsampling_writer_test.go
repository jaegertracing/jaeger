// Copyright (c) 2019 The Jaeger Authors.
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

package spanstore

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

type noopWriteSpanStore struct{}

func (*noopWriteSpanStore) WriteSpan(context.Context, *model.Span) error {
	return nil
}

var errIWillAlwaysFail = errors.New("ErrProneWriteSpanStore will always fail")

type errorWriteSpanStore struct{}

func (*errorWriteSpanStore) WriteSpan(context.Context, *model.Span) error {
	return errIWillAlwaysFail
}

// This test is to make sure downsampling works with different ratio.
func TestDownSamplingWriter_WriteSpan(t *testing.T) {
	trace := model.TraceID{
		Low:  uint64(0),
		High: uint64(1),
	}
	span := &model.Span{
		TraceID: trace,
	}
	downsamplingOptions := DownsamplingOptions{
		Ratio:    0,
		HashSalt: "jaeger-test",
	}
	c := NewDownsamplingWriter(&errorWriteSpanStore{}, downsamplingOptions)
	require.NoError(t, c.WriteSpan(context.Background(), span))

	downsamplingOptions.Ratio = 1
	c = NewDownsamplingWriter(&errorWriteSpanStore{}, downsamplingOptions)
	require.Error(t, c.WriteSpan(context.Background(), span))
}

// This test is to make sure h.hash.Reset() works and same traceID will always hash to the same value.
func TestDownSamplingWriter_hashBytes(t *testing.T) {
	downsamplingOptions := DownsamplingOptions{
		Ratio:          1,
		HashSalt:       "",
		MetricsFactory: nil,
	}
	c := NewDownsamplingWriter(&noopWriteSpanStore{}, downsamplingOptions)
	h := c.sampler.hasherPool.Get().(*hasher)
	defer c.sampler.hasherPool.Put(h)

	once := h.hashBytes()
	twice := h.hashBytes()
	assert.Equal(t, once, twice, "hashBytes should be idempotent for empty buffer")

	trace := model.TraceID{
		Low:  uint64(0),
		High: uint64(1),
	}
	span := &model.Span{
		TraceID: trace,
	}
	_, _ = span.TraceID.MarshalTo(h.buffer)
	once = h.hashBytes()
	twice = h.hashBytes()
	assert.Equal(t, once, twice, "hashBytes should be idempotent for non-empty buffer")
}

func TestDownsamplingWriter_calculateThreshold(t *testing.T) {
	var maxUint64 uint64 = math.MaxUint64
	assert.Equal(t, maxUint64, calculateThreshold(1.0))
}
