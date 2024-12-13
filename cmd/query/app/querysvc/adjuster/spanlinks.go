// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// SpanLinks creates an adjuster that removes span links with empty trace IDs.
func SpanLinks() Adjuster {
	return Func(func(traces ptrace.Traces) (ptrace.Traces, error) {
		adjuster := linksAdjuster{}
		resourceSpans := traces.ResourceSpans()
		for i := 0; i < resourceSpans.Len(); i++ {
			rs := resourceSpans.At(i)
			scopeSpans := rs.ScopeSpans()
			for j := 0; j < scopeSpans.Len(); j++ {
				ss := scopeSpans.At(j)
				spans := ss.Spans()
				for k := 0; k < spans.Len(); k++ {
					span := spans.At(k)
					adjuster.adjust(span)
				}
			}
		}
		return traces, nil
	})
}

type linksAdjuster struct{}

// adjust removes invalid links from a span.
func (l linksAdjuster) adjust(span ptrace.Span) {
	links := span.Links()
	validLinks := ptrace.NewSpanLinkSlice()
	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		if l.valid(link) {
			newLink := validLinks.AppendEmpty()
			link.CopyTo(newLink)
		}
	}
	validLinks.CopyTo(span.Links())
}

// valid checks if a span link's TraceID is not empty.
func (linksAdjuster) valid(link ptrace.SpanLink) bool {
	return !link.TraceID().IsEmpty()
}
