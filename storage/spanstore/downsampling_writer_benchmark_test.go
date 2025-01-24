// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// Benchmark result:
// BenchmarkDownSamplingWriter_WriteSpan-12    	    2000	    783766 ns/op	       1 B/op	       0 allocs/op
func BenchmarkDownSamplingWriter_WriteSpan(b *testing.B) {
	trace := model.TraceID{
		Low:  uint64(0),
		High: uint64(1),
	}
	span := &model.Span{
		TraceID: trace,
	}
	c := NewDownsamplingWriter(&noopWriteSpanStore{}, DownsamplingOptions{
		Ratio:    0.5,
		HashSalt: "jaeger-test",
	})
	b.ResetTimer()
	b.ReportAllocs()
	for it := 0; it < b.N; it++ {
		c.WriteSpan(context.Background(), span)
	}
}

// Benchmark result:
// BenchmarkDownSamplingWriter_HashBytes-12    	    5000	    381558 ns/op	       0 B/op	       0 allocs/op
func BenchmarkDownSamplingWriter_HashBytes(b *testing.B) {
	c := NewDownsamplingWriter(&noopWriteSpanStore{}, DownsamplingOptions{
		Ratio:    0.5,
		HashSalt: "jaeger-test",
	})
	ba := make([]byte, 16)
	for i := 0; i < 16; i++ {
		ba[i] = byte(i)
	}
	b.ResetTimer()
	b.ReportAllocs()
	h := c.sampler.hasherPool.Get().(*hasher)
	for it := 0; it < b.N; it++ {
		h.hashBytes()
	}
	c.sampler.hasherPool.Put(h)
}

func BenchmarkDownsamplingWriter_RandomHash(b *testing.B) {
	const numberActions = 1000000
	ratioThreshold := uint64(math.MaxUint64 / 2)
	countSmallerThanRatio := 0
	downsamplingOptions := DownsamplingOptions{
		Ratio:          1,
		HashSalt:       "jaeger-test",
		MetricsFactory: metrics.NullFactory,
	}
	c := NewDownsamplingWriter(&noopWriteSpanStore{}, downsamplingOptions)
	h := c.sampler.hasherPool.Get().(*hasher)
	for it := 0; it < b.N; it++ {
		countSmallerThanRatio = 0
		for i := 0; i < numberActions; i++ {
			low := rand.Uint64()
			high := rand.Uint64()
			span := &model.Span{
				TraceID: model.TraceID{
					Low:  low,
					High: high,
				},
			}
			_, _ = span.TraceID.MarshalTo(h.buffer[11:])
			hash := h.hashBytes()
			if hash < ratioThreshold {
				countSmallerThanRatio++
			}
		}
		fmt.Printf("Random hash ratio %f should be close to 0.5, inspect the implementation of hashBytes if not\n", math.Abs(float64(countSmallerThanRatio)/float64(numberActions)))
	}
	c.sampler.hasherPool.Put(h)
}
