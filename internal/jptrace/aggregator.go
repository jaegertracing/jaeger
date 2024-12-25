package jptrace

import (
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// AggregateTraces aggregates a sequence of trace batches into individual traces.
//
// The `tracesSeq` input must adhere to the following requirements:
//   - Each `Traces` object in the sequence must not be empty.
//   - Each `Traces` object must contain at least one span
//     (i.e., there must be at least one resource span with one scope span and one span).
func AggregateTraces(tracesSeq iter.Seq2[[]ptrace.Traces, error]) iter.Seq2[ptrace.Traces, error] {
	return func(yield func(trace ptrace.Traces, err error) bool) {
		currentTrace := ptrace.NewTraces()
		currentTraceID := pcommon.NewTraceIDEmpty()

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				yield(ptrace.NewTraces(), err)
			}
			for _, trace := range traces {
				resources := trace.ResourceSpans()
				traceID := resources.At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
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
