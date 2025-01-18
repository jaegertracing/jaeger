// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"hash/fnv"
	"unsafe"

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

func computeHashCode(hashTrace ptrace.Traces, marshaler ptrace.Marshaler) (uint64, ptrace.Span, error) {
	b, err := marshaler.MarshalTraces(hashTrace)
	if err != nil {
		return 0, ptrace.Span{}, err
	}
	hasher := fnv.New64a()
	hasher.Write(b)
	hash := hasher.Sum64()

	span := hashTrace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)

	attrs := span.Attributes()
	attrs.PutInt(jptrace.HashAttribute, uint64ToInt64Bits(hash))

	return hash, span, nil
}

func uint64ToInt64Bits(value uint64) int64 {
	return *(*int64)(unsafe.Pointer(&value))
}
