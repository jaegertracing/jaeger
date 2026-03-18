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

// AggregateTracesWithLimit aggregates a sequence of trace batches into individual traces
// but limits each trace size to maxSize spans. If maxSize is 0 or negative, there is no
// limit and all spans will be included in the aggregated trace.
//
// The `tracesSeq` input must adhere to the chunking requirements of tracestore.Reader.GetTraces.
func AggregateTracesWithLimit(tracesSeq iter.Seq2[[]ptrace.Traces, error], maxSize int) iter.Seq2[ptrace.Traces, error] {
	return func(yield func(trace ptrace.Traces, err error) bool) {
		currentTrace := ptrace.NewTraces()
		currentTraceID := pcommon.NewTraceIDEmpty()
		currentSpanCount := 0
		currentTruncated := false
		cont := true

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				cont = yield(ptrace.NewTraces(), err)
				return false
			}
			for _, trace := range traces {
				incomingSpanCount := trace.SpanCount()
				if incomingSpanCount == 0 {
					continue
				}
				traceID := GetTraceID(trace)
				if currentTraceID == traceID {
					// same trace as current, merge it into the current trace, respecting the maxSize limit
					var truncated bool
					currentSpanCount, truncated = mergeTracesWithLimit(currentTrace, currentSpanCount, trace, incomingSpanCount, maxSize)
					if truncated && !currentTruncated {
						markTraceTruncated(currentTrace, maxSize)
						currentTruncated = true
					}
				} else {
					if currentSpanCount > 0 {
						if !yield(currentTrace, nil) {
							cont = false
							return false
						}
					}
					currentTraceID = traceID
					currentTruncated = false
					if maxSize > 0 && incomingSpanCount > maxSize {
						currentTrace = ptrace.NewTraces()
						copySpansUpToLimit(currentTrace, trace, maxSize)
						currentSpanCount = maxSize
						markTraceTruncated(currentTrace, maxSize)
						currentTruncated = true
					} else {
						// Optimization: when incoming trace fits within the limit (or there is no limit),
						// we can skip the copy and use it directly as the current trace.
						currentTrace = trace
						currentSpanCount = incomingSpanCount
					}
				}
			}
			return true
		})
		// Emit the last accumulated trace if non-empty.
		// `cont` guards against calling yield after consumer already returned false.
		if cont && currentSpanCount > 0 {
			yield(currentTrace, nil)
		}
	}
}

// mergeTracesWithLimit merges src into dest, respecting the maxSize span limit.
// destCount and srcCount are the pre-computed span counts for dest and src respectively.
// Returns the updated span count of dest and whether the trace was truncated (true if
// src spans were dropped due to the limit). If maxSize <= 0, all spans are merged without limit.
func mergeTracesWithLimit(dest ptrace.Traces, destCount int, src ptrace.Traces, srcCount int, maxSize int) (int, bool) {
	// early exit if already at max
	if maxSize > 0 && destCount >= maxSize {
		return destCount, true
	}

	// check if we can merge all spans without exceeding limit
	if maxSize <= 0 || destCount+srcCount <= maxSize {
		MergeTraces(dest, src)
		return destCount + srcCount, false
	}

	// partial copy: only copy the spans that fit
	copySpansUpToLimit(dest, src, maxSize-destCount)
	return maxSize, true
}

func copySpansUpToLimit(dest, src ptrace.Traces, limit int) {
	copied := 0

	for _, srcResource := range src.ResourceSpans().All() {
		if copied >= limit {
			return
		}
		var destResource ptrace.ResourceSpans
		resourceAdded := false

		for _, srcScope := range srcResource.ScopeSpans().All() {
			if copied >= limit {
				return
			}
			var destScope ptrace.ScopeSpans
			scopeAdded := false

			for _, span := range srcScope.Spans().All() {
				if copied >= limit {
					return
				}
				// Lazily create resource and scope containers only when a span is actually copied,
				// to avoid leaving empty container artifacts in dest.
				if !resourceAdded {
					destResource = dest.ResourceSpans().AppendEmpty()
					srcResource.Resource().CopyTo(destResource.Resource())
					destResource.SetSchemaUrl(srcResource.SchemaUrl())
					resourceAdded = true
				}
				if !scopeAdded {
					destScope = destResource.ScopeSpans().AppendEmpty()
					srcScope.Scope().CopyTo(destScope.Scope())
					destScope.SetSchemaUrl(srcScope.SchemaUrl())
					scopeAdded = true
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
	for _, span := range SpanIter(trace) {
		AddWarnings(
			span,
			fmt.Sprintf("trace has more than %d spans, showing first %d spans only", maxSize, maxSize),
		)
		return // stop after first span
	}
}
