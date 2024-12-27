// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
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
	spansByHash map[uint64]*model.Span
}

func (d *spanHashDeduper) groupSpansByHash() {
	spansByHash := make(map[uint64]*model.Span)
	for _, span := range d.trace.Spans {
		hash, _ := model.HashCode(span)
		if _, ok := spansByHash[hash]; !ok {
			spansByHash[hash] = span
		}
	}
	d.spansByHash = spansByHash

	d.trace.Spans = nil
	for _, span := range d.spansByHash {
		d.trace.Spans = append(d.trace.Spans, span)
	}
}
