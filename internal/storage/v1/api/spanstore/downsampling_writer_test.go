// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics/api"
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
		MetricsFactory: api.NullFactory,
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
