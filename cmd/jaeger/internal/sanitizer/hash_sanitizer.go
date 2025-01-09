// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"fmt"
	"hash/fnv"
	"unsafe"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/model"
)

func NewHashingSanitizer() Func {
	return hashingSanitizer
}

func hashingSanitizer(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()
	marshaler := &ptrace.ProtoMarshaler{}
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
				if hashAttr, ok := span.Attributes().Get(model.SpanHashKey); ok {
					if hashAttr.Type() != pcommon.ValueTypeInt {
						jptrace.AddWarnings(span, "span hash attribute is not an integer type")
					}
				} else {
					_, updatedSpan, err := computeHashCode(hashTrace, marshaler)
					if err != nil {
						jptrace.AddWarnings(span, fmt.Sprintf("failed to compute hash code: %v", err))
						continue
					}
					updatedSpan.CopyTo(span)
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
	attrs.PutInt(model.SpanHashKey, uint64ToInt64Bits(hash))

	return hash, span, nil
}

func uint64ToInt64Bits(value uint64) int64 {
	return *(*int64)(unsafe.Pointer(&value))
}
