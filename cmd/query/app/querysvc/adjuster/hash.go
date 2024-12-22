// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"fmt"
	"hash/fnv"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
func SpanHash() SpanHashDeduper {
	return SpanHashDeduper{
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
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			newSpans := ptrace.NewSpanSlice()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				h, err := s.computeHashCode(
					span,
					rs.Resource().Attributes(),
					ss.Scope().Attributes(),
				)
				if err != nil {
					jptrace.AddWarning(span, fmt.Sprintf("failed to compute hash code: %v", err))
					span.CopyTo(newSpans.AppendEmpty())
					continue
				}
				if _, ok := spansByHash[h]; !ok {
					spansByHash[h] = span
					span.CopyTo(newSpans.AppendEmpty())
				}
			}
			newSpans.CopyTo(spans)
		}
	}
}

func (s *SpanHashDeduper) computeHashCode(
	span ptrace.Span,
	resourceAttributes pcommon.Map,
	scopeAttributes pcommon.Map,
) (uint64, error) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	resourceAttributes.CopyTo(rs.Resource().Attributes())
	ss := rs.ScopeSpans().AppendEmpty()
	scopeAttributes.CopyTo(ss.Scope().Attributes())
	newSpan := ss.Spans().AppendEmpty()
	span.CopyTo(newSpan)
	b, err := s.marshaler.MarshalTraces(traces)
	if err != nil {
		return 0, err
	}
	hasher := fnv.New64a()
	hasher.Write(b) // never returns an error
	return hasher.Sum64(), nil
}
