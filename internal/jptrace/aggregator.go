// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// AggregateTraces aggregates a sequence of trace batches into individual traces.
//
// The `tracesSeq` input must adhere to the chunking requirements of tracestore.Reader.GetTraces.
func AggregateTraces(tracesSeq iter.Seq2[[]ptrace.Traces, error]) iter.Seq2[ptrace.Traces, error] {
	return func(yield func(trace ptrace.Traces, err error) bool) {
		currentTrace := ptrace.NewTraces()
		currentTraceID := pcommon.NewTraceIDEmpty()

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				yield(ptrace.NewTraces(), err)
				return false
			}
			for _, trace := range traces {
				var (
					traceID pcommon.TraceID
					found   bool
				)
				rss := trace.ResourceSpans()
				for i := 0; i < rss.Len(); i++ {
					rs := rss.At(i)
					sss := rs.ScopeSpans()
					for j := 0; j < sss.Len(); j++ {
						ss := sss.At(j)
						sps := ss.Spans()
						if sps.Len() > 0 {
							traceID = sps.At(0).TraceID()
							found = true
							break
						}
					}
					if found {
						break
					}
				}
				if !found {
					continue
				}
				if currentTraceID == traceID {
					mergeTraces(trace, currentTrace)
				} else {
					if currentTrace.ResourceSpans().Len() > 0 {
						if !yield(currentTrace, nil) {
							return false
						}
					}
					currentTrace = trace
					currentTraceID = traceID
				}
			}
			return true
		})
		if currentTrace.ResourceSpans().Len() > 0 {
			yield(currentTrace, nil)
		}
	}
}

func mergeTraces(src, dest ptrace.Traces) {
	resources := src.ResourceSpans()
	for i := 0; i < resources.Len(); i++ {
		resource := resources.At(i)
		resource.CopyTo(dest.ResourceSpans().AppendEmpty())
	}
}
