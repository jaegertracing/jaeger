// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"errors"
	"math"

	"github.com/jaegertracing/jaeger/model"
)

// ZipkinSpanIDUniquifier returns an adjuster that changes span ids for server
// spans (i.e. spans with tag: span.kind == server) if there is another
// client span that shares the same span ID. This is needed to deal with
// Zipkin-style clients that reuse the same span ID for both client and server
// side of an RPC call. Jaeger UI expects all spans to have unique IDs.
//
// This adjuster never returns any errors. Instead it records any issues
// it encounters in Span.Warnings.
func ZipkinSpanIDUniquifier() Adjuster {
	return Func(func(trace *model.Trace) *model.Trace {
		deduper := &spanIDDeduper{trace: trace}
		deduper.groupSpansByID()
		deduper.uniquifyServerSpanIDs()
		return deduper.trace
	})
}

const (
	warningTooManySpans = "cannot assign unique span ID, too many spans in the trace"
)

var maxSpanID = model.NewSpanID(math.MaxUint64)

type spanIDDeduper struct {
	trace     *model.Trace
	spansByID map[model.SpanID][]*model.Span
	maxUsedID model.SpanID
}

// groupSpansByID groups spans with the same ID returning a map id -> []Span
func (d *spanIDDeduper) groupSpansByID() {
	spansByID := make(map[model.SpanID][]*model.Span)
	for _, span := range d.trace.Spans {
		if spans, ok := spansByID[span.SpanID]; ok {
			// TODO maybe return an error if more than 2 spans found
			spansByID[span.SpanID] = append(spans, span)
		} else {
			spansByID[span.SpanID] = []*model.Span{span}
		}
	}
	d.spansByID = spansByID
}

func (d *spanIDDeduper) isSharedWithClientSpan(spanID model.SpanID) bool {
	for _, span := range d.spansByID[spanID] {
		if span.IsRPCClient() {
			return true
		}
	}
	return false
}

func (d *spanIDDeduper) uniquifyServerSpanIDs() {
	oldToNewSpanIDs := make(map[model.SpanID]model.SpanID)
	for _, span := range d.trace.Spans {
		// only replace span IDs for server-side spans that share the ID with something else
		if span.IsRPCServer() && d.isSharedWithClientSpan(span.SpanID) {
			newID, err := d.makeUniqueSpanID()
			if err != nil {
				span.Warnings = append(span.Warnings, err.Error())
				continue
			}
			oldToNewSpanIDs[span.SpanID] = newID
			span.ReplaceParentID(span.SpanID) // previously shared ID is the new parent
			span.SpanID = newID
		}
	}
	d.swapParentIDs(oldToNewSpanIDs)
}

// swapParentIDs corrects ParentSpanID of all spans that are children of the server
// spans whose IDs we made unique.
func (d *spanIDDeduper) swapParentIDs(oldToNewSpanIDs map[model.SpanID]model.SpanID) {
	for _, span := range d.trace.Spans {
		if parentID, ok := oldToNewSpanIDs[span.ParentSpanID()]; ok {
			if span.SpanID != parentID {
				span.ReplaceParentID(parentID)
			}
		}
	}
}

// makeUniqueSpanID returns a new ID that is not used in the trace,
// or an error if such ID cannot be generated, which is unlikely,
// given that the whole space of span IDs is 2^64.
func (d *spanIDDeduper) makeUniqueSpanID() (model.SpanID, error) {
	for id := d.maxUsedID + 1; id < maxSpanID; id++ {
		if _, ok := d.spansByID[id]; !ok {
			d.maxUsedID = id
			return id, nil
		}
	}
	return 0, errors.New(warningTooManySpans)
}
