// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

const (
	invalidSpanLinkWarning = "Invalid span link removed"
)

var _ Adjuster = (*LinksAdjuster)(nil)

// SpanLinks creates an adjuster that removes span links with empty trace IDs.
func SpanLinks() LinksAdjuster {
	return LinksAdjuster{}
}

type LinksAdjuster struct{}

func (la LinksAdjuster) Adjust(traces ptrace.Traces) error {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				la.adjust(span)
			}
		}
	}
	return nil
}

// adjust removes invalid links from a span.
func (la LinksAdjuster) adjust(span ptrace.Span) {
	links := span.Links()
	validLinks := ptrace.NewSpanLinkSlice()
	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		if la.valid(link) {
			newLink := validLinks.AppendEmpty()
			link.CopyTo(newLink)
		} else {
			jptrace.AddWarning(span, invalidSpanLinkWarning)
		}
	}
	validLinks.CopyTo(span.Links())
}

// valid checks if a span link's TraceID is not empty.
func (LinksAdjuster) valid(link ptrace.SpanLink) bool {
	return !link.TraceID().IsEmpty()
}
