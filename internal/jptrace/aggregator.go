// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"fmt"
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// AggregateTraces aggregates a sequence of trace batches into individual traces.
//
// The `tracesSeq` input must adhere to the chunking requirements of tracestore.Reader.GetTraces.
func AggregateTraces(tracesSeq iter.Seq2[[]ptrace.Traces, error]) iter.Seq2[ptrace.Traces, error] {
	return AggregateTracesWithLimit(tracesSeq, 0)
}

func AggregateTracesWithLimit(tracesSeq iter.Seq2[[]ptrace.Traces, error], maxSize int) iter.Seq2[ptrace.Traces, error] {
	return func(yield func(trace ptrace.Traces, err error) bool) {
		currentTrace := ptrace.NewTraces()
		currentTraceID := pcommon.NewTraceIDEmpty()
		spanCount := 0
		cont := true

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				cont = yield(ptrace.NewTraces(), err)
				return false
			}
			for _, trace := range traces {
				if trace.SpanCount() == 0 {
					continue
				}
				resources := trace.ResourceSpans()
				traceID := resources.At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
				if currentTraceID == traceID {
					mergeTracesWithLimit(currentTrace, trace, maxSize, &spanCount)
				} else {
					if currentTrace.SpanCount() > 0 {
						if !yield(currentTrace, nil) {
							cont = false
							return false
						}
					}
					currentTrace = trace
					currentTraceID = traceID
					spanCount = trace.SpanCount()
					if maxSize > 0 && spanCount > maxSize {
						currentTrace = ptrace.NewTraces()
						spanCount = 0
						copySpansUpToLimit(currentTrace, trace, maxSize)
						spanCount = maxSize
						markTraceTruncated(currentTrace, maxSize)
					}
				}
			}
			return true
		})
		if cont && currentTrace.SpanCount() > 0 {
			yield(currentTrace, nil)
		}
	}
}

func mergeTracesWithLimit(dest, src ptrace.Traces, maxSize int, spanCount *int) bool {
	// early exit if already at max
	if maxSize > 0 && *spanCount >= maxSize {
		markTraceTruncated(dest, maxSize)
		return true
	}

	incomingCount := src.SpanCount()
	// check if we can merge all spans without exceeding limit
	if maxSize <= 0 || *spanCount+incomingCount <= maxSize {
		MergeTraces(dest, src)
		*spanCount += incomingCount
		return false
	}

	// partial copy
	remaining := maxSize - *spanCount
	if remaining > 0 {
		copySpansUpToLimit(dest, src, remaining)
		*spanCount = maxSize
	}
	markTraceTruncated(dest, maxSize)
	return true
}

func copySpansUpToLimit(dest, src ptrace.Traces, limit int) {
	copied := 0

	for _, srcResource := range src.ResourceSpans().All() {
		if copied >= limit {
			return
		}
		destResource := dest.ResourceSpans().AppendEmpty()
		srcResource.Resource().CopyTo(destResource.Resource())
		destResource.SetSchemaUrl(srcResource.SchemaUrl())

		for _, srcScope := range srcResource.ScopeSpans().All() {
			if copied >= limit {
				return
			}
			destScope := destResource.ScopeSpans().AppendEmpty()
			srcScope.Scope().CopyTo(destScope.Scope())
			destScope.SetSchemaUrl(srcScope.SchemaUrl())

			for _, span := range srcScope.Spans().All() {
				if copied >= limit {
					return
				}
				span.CopyTo(destScope.Spans().AppendEmpty())
				copied++
			}
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

func markTraceTruncated(trace ptrace.Traces, maxSize int) {
	SpanIter(trace)(func(_ SpanIterPos, span ptrace.Span) bool {
		AddWarnings(span,
			fmt.Sprintf("trace has more than %d spans, showing first %d spans only", maxSize, maxSize))
		return false // stop after first span
	})
}
