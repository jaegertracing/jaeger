// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func sequentialTrace(n int, reverse bool) ptrace.Traces {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	spans.EnsureCapacity(n + 1)
	root := spans.AppendEmpty()
	root.SetSpanID(sequentialSpanID(1))
	root.SetTraceID(pcommon.TraceID{1})
	root.SetStartTimestamp(1000)
	root.SetEndTimestamp(pcommon.Timestamp((2*n + 2) * 1000))
	for i := range n {
		child := spans.AppendEmpty()
		child.SetSpanID(sequentialSpanID(uint64(i + 2)))
		child.SetParentSpanID(root.SpanID())
		child.SetTraceID(root.TraceID())
		position := i
		if reverse {
			position = n - 1 - i
		}
		child.SetStartTimestamp(pcommon.Timestamp((2*position + 2) * 1000))
		child.SetEndTimestamp(pcommon.Timestamp((2*position + 3) * 1000))
	}
	return traces
}

func sequentialSpanID(id uint64) pcommon.SpanID {
	var result pcommon.SpanID
	binary.BigEndian.PutUint64(result[:], id)
	return result
}

func TestComputeCriticalPath_SequentialAllocations(t *testing.T) {
	const children = 512
	traces := sequentialTrace(children, false)
	// Allow generous room for trace conversion and result construction while
	// rejecting the quadratic number of temporary candidate-span allocations.
	allocations := testing.AllocsPerRun(3, func() {
		sections, err := ComputeCriticalPathFromTraces(traces)
		require.NoError(t, err)
		require.Len(t, sections, 2*children+1)
	})
	require.Less(t, allocations, float64(40*children))
}

func BenchmarkComputeCriticalPathSequential(b *testing.B) {
	for _, reverse := range []bool{false, true} {
		order := "chronological"
		if reverse {
			order = "reverse"
		}
		b.Run(order, func(b *testing.B) {
			for _, n := range []int{100, 1000, 2000, 4000, 8000} {
				b.Run(strconv.Itoa(n), func(b *testing.B) {
					traces := sequentialTrace(n, reverse)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						sections, err := ComputeCriticalPathFromTraces(traces)
						if err != nil || len(sections) != 2*n+1 {
							b.Fatalf("unexpected critical path: %d sections, error %v", len(sections), err)
						}
					}
				})
			}
		})
	}
}

func BenchmarkComputeCriticalPathShapes(b *testing.B) {
	for _, shape := range []string{"overlap", "nested"} {
		for _, n := range []int{10, 1000, 8000} {
			b.Run(shape+"/"+strconv.Itoa(n), func(b *testing.B) {
				traces := sequentialTrace(n, false)
				spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
				expectedSections := 2*n + 1
				for i := range n {
					child := spans.At(i + 1)
					if shape == "overlap" {
						child.SetStartTimestamp(2000)
						child.SetEndTimestamp(pcommon.Timestamp((i + 3) * 1000))
						expectedSections = 3
					} else {
						child.SetParentSpanID(spans.At(i).SpanID())
						child.SetStartTimestamp(pcommon.Timestamp((i + 2) * 1000))
						child.SetEndTimestamp(pcommon.Timestamp((2*n + 1 - i) * 1000))
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sections, err := ComputeCriticalPathFromTraces(traces)
					if err != nil || len(sections) != expectedSections {
						b.Fatalf("unexpected critical path: %d sections, error %v", len(sections), err)
					}
				}
			})
		}
	}
}
