// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"fmt"
	"hash/fnv"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

var _ Adjuster = (*SpanHashDeduper)(nil)

// SpanHash creates an adjuster that deduplicates spans by removing all but one span
// with the same hash code. This is particularly useful for scenarios where spans
// may be duplicated during archival, such as with ElasticSearch archival.
//
// The hash code is generated by serializing the span into protobuf bytes and applying
// the FNV hashing algorithm to the serialized data.
//
// To ensure consistent hash codes, this adjuster should be executed after
// SortAttributesAndEvents, which normalizes the order of collections within the span.
func SpanHash() *SpanHashDeduper {
	return &SpanHashDeduper{
		marshaler: &ptrace.ProtoMarshaler{},
	}
}

type SpanHashDeduper struct {
	marshaler ptrace.Marshaler
}

func (s *SpanHashDeduper) Adjust(traces ptrace.Traces) {
	spansByHash := make(map[uint64]ptrace.Span)
	resourceSpans := traces.ResourceSpans()
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
			dedupedSpans := ptrace.NewSpanSlice()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				span.CopyTo(hashSpan)
				h, err := s.computeHashCode(
					hashTrace,
				)
				if err != nil {
					jptrace.AddWarning(span, fmt.Sprintf("failed to compute hash code: %v", err))
					span.CopyTo(dedupedSpans.AppendEmpty())
					continue
				}
				if _, ok := spansByHash[h]; !ok {
					spansByHash[h] = span
					span.CopyTo(dedupedSpans.AppendEmpty())
				}
			}
			dedupedSpans.CopyTo(spans)
		}
	}
}

func (s *SpanHashDeduper) computeHashCode(
	hashTrace ptrace.Traces,
) (uint64, error) {
	b, err := s.marshaler.MarshalTraces(hashTrace)
	if err != nil {
		return 0, err
	}
	hasher := fnv.New64a()
	hasher.Write(b) // the writer in the Hash interface never returns an error
	return hasher.Sum64(), nil
}
