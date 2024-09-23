// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"github.com/jaegertracing/jaeger/model"
)

// DedupeBySpanHash returns an adjuster that removes all but one span with the same hashcode.
// This is useful for when spans are duplicated in archival storage, as happens with
// ElasticSearch archival.
func DedupeBySpanHash() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		deduper := &spanHashDeduper{trace: trace}
		deduper.groupSpansByHash()
		deduper.dedupeSpansByHash()
		return deduper.trace, nil
	})
}

type spanHashDeduper struct {
	trace       *model.Trace
	spansByHash map[uint64][]*model.Span
}

func (d *spanHashDeduper) groupSpansByHash() {
	spansByHash := make(map[uint64][]*model.Span)
	for _, span := range d.trace.Spans {
		hash, _ := model.HashCode(span)
		if spans, ok := spansByHash[hash]; ok {
			spansByHash[hash] = append(spans, span)
		} else {
			spansByHash[hash] = []*model.Span{span}
		}
	}
	d.spansByHash = spansByHash
}

func (d *spanHashDeduper) dedupeSpansByHash() {
	d.trace.Spans = nil
	for _, spans := range d.spansByHash {
		d.trace.Spans = append(d.trace.Spans, spans[0])
	}
}
