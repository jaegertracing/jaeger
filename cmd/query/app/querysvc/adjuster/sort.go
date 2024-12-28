// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"sort"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var _ Adjuster = (*SortAttributesAndEventsAdjuster)(nil)

// SortCollections creates an adjuster that standardizes trace data by sorting elements:
// - Resource attributes are sorted lexicographically by their keys.
// - Scope attributes are sorted lexicographically by their keys.
// - Span attributes are sorted lexicographically by their keys.
// - Span events are sorted lexicographically by their names.
// - Attributes within each span event are sorted lexicographically by their keys.
// - Attributes within each span link are sorted lexicographically by their keys.
func SortCollections() SortAttributesAndEventsAdjuster {
	return SortAttributesAndEventsAdjuster{}
}

type SortAttributesAndEventsAdjuster struct{}

func (s SortAttributesAndEventsAdjuster) Adjust(traces ptrace.Traces) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		resource := rs.Resource()
		s.sortAttributes(resource.Attributes())
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			s.sortAttributes(ss.Scope().Attributes())
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				s.sortAttributes(span.Attributes())
				s.sortEvents(span.Events())
				links := span.Links()
				for l := 0; l < links.Len(); l++ {
					link := links.At(l)
					s.sortAttributes(link.Attributes())
				}
			}
		}
	}
}

func (SortAttributesAndEventsAdjuster) sortAttributes(attributes pcommon.Map) {
	entries := make([]struct {
		key   string
		value pcommon.Value
	}, 0, attributes.Len())
	attributes.Range(func(k string, v pcommon.Value) bool {
		entries = append(entries, struct {
			key   string
			value pcommon.Value
		}{key: k, value: v})
		return true
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})
	newAttributes := pcommon.NewMap()
	for _, entry := range entries {
		entry.value.CopyTo(newAttributes.PutEmpty(entry.key))
	}
	newAttributes.CopyTo(attributes)
}

func (s SortAttributesAndEventsAdjuster) sortEvents(events ptrace.SpanEventSlice) {
	events.Sort(func(a, b ptrace.SpanEvent) bool {
		return a.Name() < b.Name()
	})
	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		s.sortAttributes(event.Attributes())
	}
}
