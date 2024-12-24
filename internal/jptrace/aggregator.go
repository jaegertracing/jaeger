package jptrace

import (
	"iter"
	"runtime/trace"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func AggregateTraces(tracesSeq iter.Seq2[[]ptrace.Traces, error]) iter.Seq2[ptrace.Traces, error] {
	return func(yield func(trace ptrace.Traces, err error) bool) {
		trace.IsEnabled()
		currentTrace := ptrace.NewTraces()
		currentTraceID := pcommon.NewTraceIDEmpty()

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				yield(ptrace.NewTraces(), err)
				return false
			}
			for _, trace := range traces {
				resources := trace.ResourceSpans()
				// TODO: add non-empty requirement to contract
				traceID := resources.At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
				if currentTraceID == traceID {
					// TODO: merge traces
				} else {
					if !yield(currentTrace, nil) {
						return false
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
