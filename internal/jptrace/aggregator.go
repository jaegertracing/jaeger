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
		skipCurrentTrace := false

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				yield(ptrace.NewTraces(), err)
				return false
			}
			for _, trace := range traces {
				resources := trace.ResourceSpans()
				traceID := resources.At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
				if currentTraceID == traceID {
					if !skipCurrentTrace {
						truncated := mergeTraces(trace, currentTrace, maxSize, &spanCount)
						if truncated {
							markTraceTruncated(currentTrace)
							skipCurrentTrace = true
						}
					}
				} else {
					if currentTrace.ResourceSpans().Len() > 0 {
						if !yield(currentTrace, nil) {
							return false
						}
					}
					currentTrace = trace
					currentTraceID = traceID
					spanCount = countSpans(trace)
					skipCurrentTrace = false
					if maxSize > 0 && spanCount > maxSize {
						currentTrace = ptrace.NewTraces()
						copySpansUpToLimit(trace, currentTrace, maxSize)
						spanCount = maxSize
						markTraceTruncated(currentTrace)
						skipCurrentTrace = true
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
	if maxSize <= 0 {
		// No limit, merge all
		resources := src.ResourceSpans()
		for i := 0; i < resources.Len(); i++ {
			resource := resources.At(i)
			resource.CopyTo(dest.ResourceSpans().AppendEmpty())
		}
		return false
	}

	// with limit
	incomingCount := countSpans(src)
	if *spanCount+incomingCount <= maxSize {
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
		copySpansUpToLimit(src, dest, remaining)
		*spanCount = maxSize
	}
	return true
}

func countSpans(trace ptrace.Traces) int {
	count := 0
	resources := trace.ResourceSpans()
	for i := 0; i < resources.Len(); i++ {
		scopes := resources.At(i).ScopeSpans()
		for j := 0; j < scopes.Len(); j++ {
			count += scopes.At(j).Spans().Len()
		}
	}
	return count
}

func copySpansUpToLimit(src, dest ptrace.Traces, limit int) {
	copied := 0
	resources := src.ResourceSpans()

	for i := 0; i < resources.Len() && copied < limit; i++ {
		srcResource := resources.At(i)
		destResource := dest.ResourceSpans().AppendEmpty()
		srcResource.Resource().CopyTo(destResource.Resource())
		destResource.SetSchemaUrl(srcResource.SchemaUrl())

		scopes := srcResource.ScopeSpans()
		for j := 0; j < scopes.Len() && copied < limit; j++ {
			srcScope := scopes.At(j)
			destScope := destResource.ScopeSpans().AppendEmpty()
			srcScope.Scope().CopyTo(destScope.Scope())
			destScope.SetSchemaUrl(srcScope.SchemaUrl())

			spans := srcScope.Spans()
			for k := 0; k < spans.Len() && copied < limit; k++ {
				spans.At(k).CopyTo(destScope.Spans().AppendEmpty())
				copied++
			}
		}
	}
}

func markTraceTruncated(trace ptrace.Traces) {
	resources := trace.ResourceSpans()
	if resources.Len() == 0 {
		return
	}
	scopes := resources.At(0).ScopeSpans()
	if scopes.Len() == 0 {
		return
	}
	spans := scopes.At(0).Spans()
	if spans.Len() == 0 {
		return
	}
	firstSpan := spans.At(0)
	firstSpan.Attributes().PutBool(truncationMarkerKey, true)
}

// check if a trace has marked as truncated
func IsTraceTruncated(trace ptrace.Traces) bool {
	resources := trace.ResourceSpans()
	if resources.Len() == 0 {
		return false
	}
	scopes := resources.At(0).ScopeSpans()
	if scopes.Len() == 0 {
		return false
	}
	spans := scopes.At(0).Spans()
	if spans.Len() == 0 {
		return false
	}
	firstSpan := spans.At(0)
	val, exists := firstSpan.Attributes().Get(truncationMarkerKey)
	return exists && val.Bool()
}
