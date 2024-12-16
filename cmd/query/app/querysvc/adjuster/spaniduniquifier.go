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

var _ Adjuster = (*SpanIDDeduper)(nil)

const (
	warningTooManySpans = "cannot assign unique span ID, too many spans in the trace"
)

// SpanIDUniquifier returns an adjuster that changes span ids for server
// spans (i.e. spans with tag: span.kind == server) if there is another
// client span that shares the same span ID. This is needed to deal with
// Zipkin-style clients that reuse the same span ID for both client and server
// side of an RPC call. Jaeger UI expects all spans to have unique IDs.
//
// This adjuster never returns any errors. Instead it records any issues
// it encounters in Span.Warnings.
func SpanIDUniquifier() *SpanIDDeduper {
	return &SpanIDDeduper{}
}

type SpanIDDeduper struct {
	maxUsedID pcommon.SpanID
}

func (d *SpanIDDeduper) Adjust(traces ptrace.Traces) error {
	spansByID := make(map[pcommon.SpanID][]ptrace.Span)
	d.maxUsedID = pcommon.NewSpanIDEmpty() // reset maxUsedID for each trace
	d.groupSpansByID(traces, spansByID)
	d.uniquifyServerSpanIDs(traces, spansByID)
	return nil
}

// groupSpansByID groups spans with the same ID returning a map id -> []Span
func (*SpanIDDeduper) groupSpansByID(traces ptrace.Traces, spansByID map[pcommon.SpanID][]ptrace.Span) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if spans, ok := spansByID[span.SpanID()]; ok {
					// TODO maybe return an error if more than 2 spans found
					spansByID[span.SpanID()] = append(spans, span)
				} else {
					spansByID[span.SpanID()] = []ptrace.Span{span}
				}
			}
		}
	}
}

func (*SpanIDDeduper) isSharedWithClientSpan(spanID pcommon.SpanID, spansByID map[pcommon.SpanID][]ptrace.Span) bool {
	spans := spansByID[spanID]
	for _, span := range spans {
		if span.Kind() == ptrace.SpanKindClient {
			return true
		}
	}
	return false
}

func (d *SpanIDDeduper) uniquifyServerSpanIDs(traces ptrace.Traces, spansByID map[pcommon.SpanID][]ptrace.Span) {
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
				if span.Kind() == ptrace.SpanKindServer && d.isSharedWithClientSpan(span.SpanID(), spansByID) {
					newID, err := d.makeUniqueSpanID(spansByID)
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
func (*SpanIDDeduper) swapParentIDs(
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
func (d *SpanIDDeduper) makeUniqueSpanID(spansByID map[pcommon.SpanID][]ptrace.Span) (pcommon.SpanID, error) {
	id := incrementSpanID(d.maxUsedID)
	for id != pcommon.NewSpanIDEmpty() {
		if _, exists := spansByID[id]; !exists {
			d.maxUsedID = id
			return id, nil
		}
		id = incrementSpanID(id)
	}
	return pcommon.NewSpanIDEmpty(), errors.New(warningTooManySpans)
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
