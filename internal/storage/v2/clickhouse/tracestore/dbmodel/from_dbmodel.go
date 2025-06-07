// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/hex"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func FromDBModel(storedSpan Span) ptrace.Traces {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()

	sp, err := convertSpan(storedSpan)
	sp.CopyTo(span)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}

	resource := resourceSpans.Resource()
	rs, err := convertResource(storedSpan)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}
	rs.CopyTo(resource)

	scope := scopeSpans.Scope()
	sc, err := convertScope(storedSpan)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}
	sc.CopyTo(scope)

	for i := range storedSpan.Events {
		event := span.Events().AppendEmpty()
		e, err := convertEvent(storedSpan.Events[i])
		if err != nil {
			jptrace.AddWarnings(span, err.Error())
		}
		e.CopyTo(event)
	}

	for i := range storedSpan.Links {
		link := span.Links().AppendEmpty()
		l, err := convertSpanLink(storedSpan.Links[i])
		if err != nil {
			jptrace.AddWarnings(span, err.Error())
		}
		l.CopyTo(link)
	}
	return trace
}

func convertResource(Span) (pcommon.Resource, error) {
	resource := ptrace.NewResourceSpans().Resource()
	// TODO: populate attributes
	// TODO: do we populate the service name from the span?
	return resource, nil
}

func convertScope(s Span) (pcommon.InstrumentationScope, error) {
	scope := ptrace.NewScopeSpans().Scope()
	scope.SetName(s.ScopeName)
	scope.SetVersion(s.ScopeVersion)
	// TODO: populate attributes

	return scope, nil
}

func convertSpan(s Span) (ptrace.Span, error) {
	span := ptrace.NewSpan()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(s.StartTime))
	traceId, err := hex.DecodeString(s.TraceID)
	if err != nil {
		return span, fmt.Errorf("failed to decode trace ID: %w", err)
	}
	span.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(s.ID)
	if err != nil {
		return span, fmt.Errorf("failed to decode span ID: %w", err)
	}
	span.SetSpanID(pcommon.SpanID(spanId))
	parentSpanId, err := hex.DecodeString(s.ParentSpanID)
	if err != nil {
		return span, fmt.Errorf("failed to decode parent span ID: %w", err)
	}
	if len(parentSpanId) != 0 {
		span.SetParentSpanID(pcommon.SpanID(parentSpanId))
	}
	span.TraceState().FromRaw(s.TraceState)
	span.SetName(s.Name)
	span.SetKind(convertSpanKind(s.Kind))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(s.StartTime.Add(s.Duration)))
	span.Status().SetCode(convertStatusCode(s.StatusCode))
	span.Status().SetMessage(s.StatusMessage)
	// TODO: populate attributes

	return span, nil
}

func convertEvent(e Event) (ptrace.SpanEvent, error) {
	event := ptrace.NewSpanEvent()
	event.SetName(e.Name)
	event.SetTimestamp(pcommon.NewTimestampFromTime(e.Timestamp))

	// TODO: populate attributes
	return event, nil
}

func convertSpanLink(l Link) (ptrace.SpanLink, error) {
	link := ptrace.NewSpanLink()
	traceId, err := hex.DecodeString(l.TraceID)
	if err != nil {
		return link, fmt.Errorf("failed to decode link trace ID: %w", err)
	}
	link.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(l.SpanID)
	if err != nil {
		return link, fmt.Errorf("failed to decode link span ID: %w", err)
	}
	link.SetSpanID(pcommon.SpanID(spanId))
	link.TraceState().FromRaw(l.TraceState)
	// TODO: populate attributes
	return link, nil
}

func convertSpanKind(sk string) ptrace.SpanKind {
	switch sk {
	case "Unspecified":
		return ptrace.SpanKindUnspecified
	case "Internal":
		return ptrace.SpanKindInternal
	case "Server":
		return ptrace.SpanKindServer
	case "Client":
		return ptrace.SpanKindClient
	case "Producer":
		return ptrace.SpanKindProducer
	case "Consumer":
		return ptrace.SpanKindConsumer
	}
	return ptrace.SpanKindUnspecified
}

func convertStatusCode(sc string) ptrace.StatusCode {
	switch sc {
	case "OK":
		return ptrace.StatusCodeOk
	case "Unset":
		return ptrace.StatusCodeUnset
	case "Error":
		return ptrace.StatusCodeError
	}
	return ptrace.StatusCodeUnset
}
