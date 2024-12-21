// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"hash/fnv"

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
	return SpanHashDeduper{}
}

type SpanHashDeduper struct{}

func (s *SpanHashDeduper) Adjust(traces ptrace.Traces) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spansByHash := make(map[uint64]ptrace.Span)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				h, err := s.computeHashCode(span)
				if err != nil {
					// TODO: what should we do here?
					continue
				}
				if _, ok := spansByHash[h]; !ok {
					spansByHash[h] = span
				}
			}
			newSpans := ptrace.NewSpanSlice()
			for _, span := range spansByHash {
				span.CopyTo(newSpans.AppendEmpty())
			}
			newSpans.CopyTo(ss.Spans())
		}
	}
}

func (s *SpanHashDeduper) computeHashCode(span ptrace.Span) (uint64, error) {
	traces := ptrace.NewTraces()
	newSpan := traces.
		ResourceSpans().
		AppendEmpty().
		ScopeSpans().
		AppendEmpty().
		Spans().
		AppendEmpty()
	span.CopyTo(newSpan)
	marshaller := ptrace.ProtoMarshaler{}
	b, err := marshaller.MarshalTraces(traces)
	if err != nil {
		return 0, err
	}
	hasher := fnv.New64a()
	_, err = hasher.Write(b)
	if err != nil {
		return 0, err
	}
	return hasher.Sum64(), nil
}
