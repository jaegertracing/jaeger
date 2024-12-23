// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"errors"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

var errTooManySpans = errors.New("cannot assign unique span ID, too many spans in the trace")

// DeduplicateClientServerSpanIDs returns an adjuster that changes span ids for server
// spans (i.e. spans with tag: span.kind == server) if there is another
// client span that shares the same span ID. This is needed to deal with
// Zipkin-style clients that reuse the same span ID for both client and server
// side of an RPC call. Jaeger UI expects all spans to have unique IDs.
//
// Any issues encountered during adjustment are recorded as warnings in the
// span.
func DeduplicateClientServerSpanIDs() Adjuster {
	return Func(func(traces ptrace.Traces) {
		adjuster := spanIDDeduper{
			spansByID: make(map[pcommon.SpanID][]ptrace.Span),
			maxUsedID: pcommon.NewSpanIDEmpty(),
		}
		adjuster.adjust(traces)
	})
}

type spanIDDeduper struct {
	spansByID map[pcommon.SpanID][]ptrace.Span
	maxUsedID pcommon.SpanID
}

func (d *spanIDDeduper) adjust(traces ptrace.Traces) {
	d.groupSpansByID(traces)
	d.uniquifyServerSpanIDs(traces)
}

// groupSpansByID groups spans with the same ID returning a map id -> []Span
func (d *spanIDDeduper) groupSpansByID(traces ptrace.Traces) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if spans, ok := d.spansByID[span.SpanID()]; ok {
					d.spansByID[span.SpanID()] = append(spans, span)
				} else {
					d.spansByID[span.SpanID()] = []ptrace.Span{span}
				}
			}
		}
	}
}

func (d *spanIDDeduper) isSharedWithClientSpan(spanID pcommon.SpanID) bool {
	spans := d.spansByID[spanID]
	for _, span := range spans {
		if span.Kind() == ptrace.SpanKindClient {
			return true
		}
	}
	return false
}

func (d *spanIDDeduper) uniquifyServerSpanIDs(traces ptrace.Traces) {
	oldToNewSpanIDs := make(map[pcommon.SpanID]pcommon.SpanID)
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				// only replace span IDs for server-side spans that share the ID with something else
				if span.Kind() == ptrace.SpanKindServer && d.isSharedWithClientSpan(span.SpanID()) {
					newID, err := d.makeUniqueSpanID()
					if err != nil {
						jptrace.AddWarning(span, err.Error())
						continue
					}
					oldToNewSpanIDs[span.SpanID()] = newID
					span.SetParentSpanID(span.SpanID()) // previously shared ID is the new parent
					span.SetSpanID(newID)
				}
			}
		}
	}
	d.swapParentIDs(traces, oldToNewSpanIDs)
}

// swapParentIDs corrects ParentSpanID of all spans that are children of the server
// spans whose IDs we made unique.
func (*spanIDDeduper) swapParentIDs(
	traces ptrace.Traces,
	oldToNewSpanIDs map[pcommon.SpanID]pcommon.SpanID,
) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if parentID, ok := oldToNewSpanIDs[span.ParentSpanID()]; ok {
					if span.SpanID() != parentID {
						span.SetParentSpanID(parentID)
					}
				}
			}
		}
	}
}

// makeUniqueSpanID returns a new ID that is not used in the trace,
// or an error if such ID cannot be generated, which is unlikely,
// given that the whole space of span IDs is 2^64.
func (d *spanIDDeduper) makeUniqueSpanID() (pcommon.SpanID, error) {
	id := incrementSpanID(d.maxUsedID)
	for id != pcommon.NewSpanIDEmpty() {
		if _, exists := d.spansByID[id]; !exists {
			d.maxUsedID = id
			return id, nil
		}
		id = incrementSpanID(id)
	}
	return pcommon.NewSpanIDEmpty(), errTooManySpans
}

func incrementSpanID(spanID pcommon.SpanID) pcommon.SpanID {
	newID := spanID
	for i := len(newID) - 1; i >= 0; i-- {
		newID[i]++
		if newID[i] != 0 {
			break
		}
	}
	return newID
}
