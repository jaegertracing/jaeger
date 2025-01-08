// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"unsafe"

	"github.com/jaegertracing/jaeger/model"
)

// DedupeBySpanHash returns an adjuster that removes all but one span with the same hashcode.
// This is useful for when spans are duplicated in archival storage, as happens with
// ElasticSearch archival.
func DedupeBySpanHash() Adjuster {
	return Func(func(trace *model.Trace) {
		deduper := &spanHashDeduper{trace: trace}
		deduper.groupSpansByHash()
	})
}

type spanHashDeduper struct {
	trace       *model.Trace
	spansByHash map[int64]*model.Span
}

func (d *spanHashDeduper) groupSpansByHash() {
	spansByHash := make(map[int64]*model.Span)
	for _, span := range d.trace.Spans {
		spanHash, found := span.GetHashTag()
		if !found {
			hash, _ := model.HashCode(span)
			spanHash = uint64ToInt64Bits(hash)
		}

		if _, ok := spansByHash[spanHash]; !ok {
			spansByHash[spanHash] = span
		}
	}
	d.spansByHash = spansByHash

	d.trace.Spans = nil
	for _, span := range d.spansByHash {
		d.trace.Spans = append(d.trace.Spans, span)
	}
}

func uint64ToInt64Bits(value uint64) int64 {
	return *(*int64)(unsafe.Pointer(&value))
}
