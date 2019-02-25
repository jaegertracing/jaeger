package spanstore

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/model"
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
	c := NewDownSamplingWriter(&noopWriteSpanStore{}, DownSamplingOptions{
		Ratio:    0.5,
		HashSalt: "jaeger-test",
	})

	b.ResetTimer()
	b.ReportAllocs()
	for it := 0; it < b.N; it++ {
		for writes := 0; writes < 10000; writes++ {
			c.WriteSpan(span)
		}

	}
}

// Benchmark result:
// BenchmarkDownSamplingWriter_HashBytes-12    	    5000	    381558 ns/op	       0 B/op	       0 allocs/op
func BenchmarkDownSamplingWriter_HashBytes(b *testing.B) {
	c := NewDownSamplingWriter(&noopWriteSpanStore{}, DownSamplingOptions{
		Ratio:    0.5,
		HashSalt: "jaeger-test",
	})
	ba := make([]byte, 16)
	b.ResetTimer()
	b.ReportAllocs()

	for it := 0; it < b.N; it++ {
		for i := 0; i < 10000; i++ {
			c.hashBytes(ba)
		}
	}
}

func BenchmarkDownSamplingWriter_RandomHash(b *testing.B) {
	ratioThreshold := uint64(math.MaxUint64 / 2)
	countSmallerThanRatio := 0
	downSamplingOptions := DownSamplingOptions{
		Ratio:          1,
		HashSalt:       "jaeger-test",
		MetricsFactory: metrics.NullFactory,
	}
	c := NewDownSamplingWriter(&noopWriteSpanStore{}, downSamplingOptions)
	for it := 0; it < b.N; it++ {
		countSmallerThanRatio = 0
		for i := 0; i < 1000000; i++ {
			low := rand.Uint64()
			high := rand.Uint64()
			span := &model.Span{
				TraceID: model.TraceID{
					Low:  low,
					High: high,
				},
			}
			hash := c.hashBytes([]byte(span.TraceID.String()))
			if hash < ratioThreshold {
				countSmallerThanRatio++
			}
		}
		fmt.Printf("Random hash ratio %f should be close to 0.5, inspect the implementation of hashBytes if not\n", math.Abs(float64(countSmallerThanRatio)/1000000))
	}

}
