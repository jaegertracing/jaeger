// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package adjuster

import (
	"errors"

	"github.com/uber/jaeger/model"
)

// SpanIDDeduper returns an adjuster that changes span ids for server
// spans (i.e. spans with tag: span.kind == server) if there is another
// client span that shares the same span ID. This is needed to deal with
// Zipkin-style clients that reuse the same span ID for both client and server
// side of an RPC call. Jaeger UI expects all spans to have unique IDs.
func SpanIDDeduper() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		deduper := &spanIDDeduper{trace: trace}
		deduper.groupByID()
		err := deduper.dedupeSpanIDs()
		return deduper.trace, err
	})
}

var errTooManySpans = errors.New("cannot assign unique span ID, too many spans in the trace")

const maxSpanID = model.SpanID(0xffffffffffffffff)

type spanIDDeduper struct {
	trace     *model.Trace
	spansByID map[model.SpanID][]*model.Span
	maxUsedID model.SpanID
}

// groupByID groups spans with the same ID returning a map id -> []Span
func (d *spanIDDeduper) groupByID() {
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

func (d *spanIDDeduper) dedupeSpanIDs() error {
	oldToNewSpanIDs := make(map[model.SpanID]model.SpanID)
	for _, span := range d.trace.Spans {
		// only replace span IDs for server-side spans that share the ID with something else
		if span.IsRPCServer() && d.isSharedWithClientSpan(span.SpanID) {
			newID, err := d.makeUniqueSpanID()
			if err != nil {
				return err
			}
			oldToNewSpanIDs[span.SpanID] = newID
			span.ParentSpanID = span.SpanID // previously shared ID is the new parent
			span.SpanID = newID
		}
	}
	d.swapParentIDs(oldToNewSpanIDs)
	return nil
}

// swapParentIDs corrects ParentSpanID of all spans that are children of the server
// spans whose IDs we deduped.
func (d *spanIDDeduper) swapParentIDs(oldToNewSpanIDs map[model.SpanID]model.SpanID) {
	for _, span := range d.trace.Spans {
		if parentID, ok := oldToNewSpanIDs[span.ParentSpanID]; ok {
			if span.SpanID != parentID {
				span.ParentSpanID = parentID
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
	return 0, errTooManySpans
}
