// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func NewHashingSanitizer() Func {
	return hashingSanitizer
}

func hashingSanitizer(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()
	spanHasher := jptrace.NewSpanHasher()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		hashTrace := ptrace.NewTraces()
		hashResourceSpan := hashTrace.ResourceSpans().AppendEmpty()
		hashScopeSpan := hashResourceSpan.ScopeSpans().AppendEmpty()
		hashSpan := hashScopeSpan.Spans().AppendEmpty()
		rs.Resource().Attributes().CopyTo(hashResourceSpan.Resource().Attributes())
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			ss.Scope().Attributes().CopyTo(hashScopeSpan.Scope().Attributes())
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				span.CopyTo(hashSpan)
				spanHash, err := spanHasher.SpanHash(hashTrace)
				if err != nil {
					jptrace.AddWarnings(span, err.Error())
				}
				if spanHash != 0 {
					attrs := span.Attributes()
					attrs.PutInt(jptrace.HashAttribute, spanHash)
				}
			}
		}
	}

	return traces
}
