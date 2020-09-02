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

	"github.com/jaegertracing/jaeger/model"
)

type noopWriteSpanStore struct{}

func (n *noopWriteSpanStore) WriteSpan(ct context.Context, span *model.Span) error {
	return nil
}

var errIWillAlwaysFail = errors.New("ErrProneWriteSpanStore will always fail")

type errorWriteSpanStore struct{}

func (n *errorWriteSpanStore) WriteSpan(ctx context.Context, span *model.Span) error {
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
	assert.NoError(t, c.WriteSpan(context.Background(), span))

	downsamplingOptions.Ratio = 1
	c = NewDownsamplingWriter(&errorWriteSpanStore{}, downsamplingOptions)
	assert.Error(t, c.WriteSpan(context.Background(), span))
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
	assert.Equal(t, h.hashBytes(), h.hashBytes())
	c.sampler.hasherPool.Put(h)
	trace := model.TraceID{
		Low:  uint64(0),
		High: uint64(1),
	}
	span := &model.Span{
		TraceID: trace,
	}
	_, _ = span.TraceID.MarshalTo(h.buffer)
	// Same traceID should always be hashed to same uint64 in DownSamplingWriter.
	assert.Equal(t, h.hashBytes(), h.hashBytes())
}

func TestDownsamplingWriter_calculateThreshold(t *testing.T) {
	var maxUint64 uint64 = math.MaxUint64
	assert.Equal(t, maxUint64, calculateThreshold(1.0))
}
