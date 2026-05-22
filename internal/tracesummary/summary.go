// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracesummary

import (
	"sort"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

const serviceNameAttr = "service.name"

// FromTrace derives a compact summary from a single trace.
func FromTrace(trace ptrace.Traces) tracestore.TraceSummary {
	var b builder
	b.services = make(map[string]*tracestore.ServiceSummary)
	b.spanIDs = make(map[pcommon.SpanID]struct{})

	for _, span := range jptrace.SpanIter(trace) {
		b.spanIDs[span.SpanID()] = struct{}{}
	}
	for pos, span := range jptrace.SpanIter(trace) {
		b.observeSpan(pos.Resource.Resource().Attributes(), span)
	}

	return b.build()
}

type builder struct {
	summary             tracestore.TraceSummary
	services            map[string]*tracestore.ServiceSummary
	spanIDs             map[pcommon.SpanID]struct{}
	rootSpan            ptrace.Span
	rootServiceName     string
	hasRootSpan         bool
	fallbackSpan        ptrace.Span
	fallbackServiceName string
	hasFallbackSpan     bool
	hasStartTime        bool
	startTime           time.Time
	endTime             time.Time
}

func (b *builder) observeSpan(resourceAttrs pcommon.Map, span ptrace.Span) {
	serviceName := serviceName(resourceAttrs)
	if b.summary.TraceID.IsEmpty() {
		b.summary.TraceID = span.TraceID()
	}
	b.summary.SpanCount++
	if span.Status().Code() == ptrace.StatusCodeError {
		b.summary.ErrorSpanCount++
	}

	breakdown := b.services[serviceName]
	if breakdown == nil {
		breakdown = &tracestore.ServiceSummary{Name: serviceName}
		b.services[serviceName] = breakdown
	}
	breakdown.SpanCount++
	if span.Status().Code() == ptrace.StatusCodeError {
		breakdown.ErrorSpanCount++
	}

	start := span.StartTimestamp().AsTime()
	end := span.EndTimestamp().AsTime()
	if !b.hasStartTime || start.Before(b.startTime) {
		b.startTime = start
		b.hasStartTime = true
	}
	if end.After(b.endTime) {
		b.endTime = end
	}

	if b.isBetterRoot(span) {
		b.rootSpan = span
		b.rootServiceName = serviceName
		b.hasRootSpan = true
	}
	if b.isBetterFallback(span) {
		b.fallbackSpan = span
		b.fallbackServiceName = serviceName
		b.hasFallbackSpan = true
	}
	if b.isOrphan(span) {
		b.summary.OrphanSpanCount++
	}
}

func (b *builder) isBetterRoot(span ptrace.Span) bool {
	if !b.isRootCandidate(span) {
		return false
	}
	if !b.hasRootSpan {
		return true
	}
	rootStart := b.rootSpan.StartTimestamp()
	spanStart := span.StartTimestamp()
	if spanStart != rootStart {
		return spanStart < rootStart
	}
	return span.SpanID().String() < b.rootSpan.SpanID().String()
}

func (b *builder) isBetterFallback(span ptrace.Span) bool {
	if !b.hasFallbackSpan {
		return true
	}
	fallbackStart := b.fallbackSpan.StartTimestamp()
	spanStart := span.StartTimestamp()
	if spanStart != fallbackStart {
		return spanStart < fallbackStart
	}
	return span.SpanID().String() < b.fallbackSpan.SpanID().String()
}

func (b *builder) isRootCandidate(span ptrace.Span) bool {
	parentSpanID := span.ParentSpanID()
	if parentSpanID.IsEmpty() {
		return true
	}
	return b.isOrphan(span)
}

func (b *builder) isOrphan(span ptrace.Span) bool {
	parentSpanID := span.ParentSpanID()
	if parentSpanID.IsEmpty() {
		return false
	}
	_, parentInTrace := b.spanIDs[parentSpanID]
	return !parentInTrace
}

func (b *builder) build() tracestore.TraceSummary {
	if b.hasRootSpan {
		b.summary.RootServiceName = b.rootServiceName
		b.summary.RootOperationName = b.rootSpan.Name()
	} else if b.hasFallbackSpan {
		b.summary.RootServiceName = b.fallbackServiceName
		b.summary.RootOperationName = b.fallbackSpan.Name()
	}
	if b.hasStartTime {
		b.summary.MinStartTime = b.startTime
		b.summary.MaxEndTime = b.endTime
	}

	b.summary.Services = make([]tracestore.ServiceSummary, 0, len(b.services))
	for _, breakdown := range b.services {
		b.summary.Services = append(b.summary.Services, *breakdown)
	}
	sort.Slice(b.summary.Services, func(i, j int) bool {
		return b.summary.Services[i].Name < b.summary.Services[j].Name
	})
	return b.summary
}

func serviceName(attrs pcommon.Map) string {
	if value, ok := attrs.Get(serviceNameAttr); ok {
		return value.AsString()
	}
	return ""
}
