// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/pkg/iter"
)

type SpanIterPos struct {
	Resource      ptrace.ResourceSpans
	ResourceIndex int
	Scope         ptrace.ScopeSpans
	ScopeIndex    int
}

// SpanIter iterates over all spans in the provided ptrace.Traces and yields each span.
func SpanIter(traces ptrace.Traces) iter.Seq2[SpanIterPos, ptrace.Span] {
	return func(yield func(SpanIterPos, ptrace.Span) bool) {
		var pos SpanIterPos
		for i := 0; i < traces.ResourceSpans().Len(); i++ {
			resource := traces.ResourceSpans().At(i)
			pos.Resource = resource
			pos.ResourceIndex = i
			for j := 0; j < resource.ScopeSpans().Len(); j++ {
				scope := resource.ScopeSpans().At(j)
				pos.Scope = scope
				pos.ScopeIndex = j
				for k := 0; k < scope.Spans().Len(); k++ {
					span := scope.Spans().At(k)
					if !yield(pos, span) {
						return
					}
				}
			}
		}
	}
}
