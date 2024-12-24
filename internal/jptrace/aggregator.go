package jptrace

import (
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func AggregateTraces(tracesSeq iter.Seq[[]ptrace.Traces]) iter.Seq[ptrace.Traces] {
	return func(yield func(trace ptrace.Traces) bool) {
		currentTrace := ptrace.NewTraces()
		currentTraceID := pcommon.NewTraceIDEmpty()

		tracesSeq(func(traces []ptrace.Traces) bool {
			for _, trace := range traces {
				resources := trace.ResourceSpans()
				// TODO: add non-empty requirement to contract
				traceID := resources.At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
				if currentTraceID == pcommon.NewTraceIDEmpty() {
					currentTrace = trace
					currentTraceID = traceID
				} else if currentTraceID == traceID {
					// TODO: merge traces
				} else {
					if !yield(currentTrace) {
						return false
					}
					currentTrace = ptrace.NewTraces()
					currentTraceID = pcommon.NewTraceIDEmpty()
				}
			}
			return true
		})

		if currentTraceID != pcommon.NewTraceIDEmpty() {
			yield(currentTrace)
		}
	}
}
