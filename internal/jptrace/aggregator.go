// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const truncationMarkerKey = "@jaeger@truncated"

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

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				yield(ptrace.NewTraces(), err)
				return false
			}
			for _, trace := range traces {
				resources := trace.ResourceSpans()
				traceID := resources.At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
				if currentTraceID == traceID {
					truncated := mergeTraces(trace, currentTrace, maxSize, &spanCount)
					if truncated {
						markTraceTruncated(currentTrace)
					}
				} else {
					if currentTrace.ResourceSpans().Len() > 0 {
						if !yield(currentTrace, nil) {
							return false
						}
					}
					currentTrace = trace
					currentTraceID = traceID
					spanCount = trace.SpanCount()
					if maxSize > 0 && spanCount > maxSize {
						currentTrace = ptrace.NewTraces()
						copySpansUpToLimit(currentTrace, trace, maxSize)
						spanCount = maxSize
						markTraceTruncated(currentTrace)
					}
				}
			}
			return true
		})
		if currentTrace.ResourceSpans().Len() > 0 {
			yield(currentTrace, nil)
		}
	}
}

func mergeTraces(src, dest ptrace.Traces, maxSize int, spanCount *int) bool {
	// early exit if already at max
	if maxSize > 0 && *spanCount >= maxSize {
		return true
	}

	incomingCount := src.SpanCount()
	// check if we can merge all spans without exceeding limit
	if maxSize <= 0 || *spanCount+incomingCount <= maxSize {
		resources := src.ResourceSpans()
		for i := 0; i < resources.Len(); i++ {
			resource := resources.At(i)
			resource.CopyTo(dest.ResourceSpans().AppendEmpty())
		}
		*spanCount += incomingCount
		return false
	}

	// partial copy
	remaining := maxSize - *spanCount
	if remaining > 0 {
		copySpansUpToLimit(dest, src, remaining)
		*spanCount = maxSize
	}
	return true
}

func copySpansUpToLimit(dest, src ptrace.Traces, limit int) {
	copied := 0

	for _, srcResource := range src.ResourceSpans().All() {
		destResource := dest.ResourceSpans().AppendEmpty()
		srcResource.Resource().CopyTo(destResource.Resource())
		destResource.SetSchemaUrl(srcResource.SchemaUrl())

		for _, srcScope := range srcResource.ScopeSpans().All() {
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

func markTraceTruncated(trace ptrace.Traces) {
	// direct access to first span (if truncated, it must exist)
	firstSpan := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	firstSpan.Attributes().PutBool(truncationMarkerKey, true)
}

// check if a trace has marked as truncated
func IsTraceTruncated(trace ptrace.Traces) bool {
	for i := 0; i < trace.ResourceSpans().Len(); i++ {
		scopes := trace.ResourceSpans().At(i).ScopeSpans()
		for j := 0; j < scopes.Len(); j++ {
			spans := scopes.At(j).Spans()
			if spans.Len() > 0 {
				val, exists := spans.At(0).Attributes().Get(truncationMarkerKey)
				return exists && val.Bool()
			}
		}
	}
	return false
}
