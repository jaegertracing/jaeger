// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"github.com/jaegertracing/jaeger/model"
)

// DedupeBySpanID returns an adjuster that removes all but one span with the same SpanID.
// This is useful for when spans are duplicated in archival storage, as happens with
// ElasticSearch archival.
func DedupeBySpanID() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		deduper := &spanIDDeduper{trace: trace}
		deduper.groupSpansByID()
		deduper.dedupeSpansByID()
		return deduper.trace, nil
	})
}

func (d *spanIDDeduper) dedupeSpansByID() {
	d.trace.Spans = nil
	for _, spans := range d.spansByID {
		d.trace.Spans = append(d.trace.Spans, spans[0])
	}
}
