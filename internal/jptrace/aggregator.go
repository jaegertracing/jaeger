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
				if trace.SpanCount() == 0 {
					continue
				}
				resources := trace.ResourceSpans()
				traceID := resources.At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
				if currentTraceID == traceID {
					MergeTraces(currentTrace, trace)
				} else {
					if currentTrace.SpanCount() > 0 {
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
		if currentTrace.SpanCount() > 0 {
			yield(currentTrace, nil)
		}
	}
}

// MergeTraces merges src trace into dest trace.
// This is useful when multiple iterations return parts of the same trace.
func MergeTraces(dest, src ptrace.Traces) {
	resources := src.ResourceSpans()
	for i := 0; i < resources.Len(); i++ {
		resource := resources.At(i)
		resource.CopyTo(dest.ResourceSpans().AppendEmpty())
	}
}
