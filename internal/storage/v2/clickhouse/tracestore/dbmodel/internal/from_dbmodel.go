// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// FromDBModel convert the Trace to ptrace.Traces
func FromDBModel(dbTrace Trace) ptrace.Traces {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()

	resourceAttributes := attributesGroupToMap(dbTrace.Resource.Attributes)
	resourceAttributes.CopyTo(resourceSpans.Resource().Attributes())

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	scope := scopeSpans.Scope()
	FromDBScope(dbTrace.Scope).CopyTo(scope)

	span := scopeSpans.Spans().AppendEmpty()
	FromDBSpan(dbTrace.Span).CopyTo(span)
	for i := range dbTrace.Events {
		event := span.Events().AppendEmpty()
		fromDBEvent(dbTrace.Events[i]).CopyTo(event)
	}

	for i := range dbTrace.Links {
		link := span.Links().AppendEmpty()
		fromDBLink(dbTrace.Links[i]).CopyTo(link)
	}
	return trace
}

func FromDBScope(s Scope) pcommon.InstrumentationScope {
	scope := ptrace.NewScopeSpans().Scope()
	scope.SetName(s.Name)
	scope.SetVersion(s.Version)
	attributes := attributesGroupToMap(s.Attributes)
	attributes.CopyTo(scope.Attributes())

	return scope
}

func FromDBSpan(s Span) ptrace.Span {
	span := ptrace.NewSpan()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(s.StartTime))
	span.SetTraceID(pcommon.TraceID(s.TraceId))
	span.SetSpanID(pcommon.SpanID(s.SpanId))
	span.SetParentSpanID(pcommon.SpanID(s.ParentSpanId))
	span.TraceState().FromRaw(s.TraceState)
	span.SetName(s.Name)
	span.SetKind(fromDBSpanKind(s.Kind))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(s.StartTime.Add(time.Duration(s.Duration))))
	span.Status().SetCode(fromDBStatusCode(s.StatusCode))
	span.Status().SetMessage(s.StatusMessage)
	spanAttributes := attributesGroupToMap(s.Attributes)
	spanAttributes.CopyTo(span.Attributes())

	return span
}

func fromDBEvent(e Event) ptrace.SpanEvent {
	event := ptrace.NewSpanEvent()
	event.SetName(e.Name)
	event.SetTimestamp(pcommon.NewTimestampFromTime(e.Timestamp))

	attributes := attributesGroupToMap(e.Attributes)
	attributes.CopyTo(event.Attributes())
	return event
}

func fromDBLink(l Link) ptrace.SpanLink {
	link := ptrace.NewSpanLink()

	link.SetTraceID(pcommon.TraceID(l.TraceId))
	link.SetSpanID(pcommon.SpanID(l.SpanId))
	link.TraceState().FromRaw(l.TraceState)
	attributes := attributesGroupToMap(l.Attributes)
	attributes.CopyTo(link.Attributes())
	return link
}

func fromDBSpanKind(sk string) ptrace.SpanKind {
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

func fromDBStatusCode(sc string) ptrace.StatusCode {
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

func attributesGroupToMap(group AttributesGroup) pcommon.Map {
	result := pcommon.NewMap()
	for i := range group.BoolKeys {
		key := group.BoolKeys[i]
		value := group.BoolValues[i]
		result.PutBool(key, value)
	}
	for i := range group.IntKeys {
		key := group.IntKeys[i]
		value := group.IntValues[i]
		result.PutInt(key, value)
	}
	for i := range group.DoubleKeys {
		key := group.DoubleKeys[i]
		value := group.DoubleValues[i]
		result.PutDouble(key, value)
	}
	for i := range group.StrKeys {
		key := group.StrKeys[i]
		value := group.StrValues[i]
		result.PutStr(key, value)
	}
	for i := range group.BytesKeys {
		key := group.BytesKeys[i]
		value := group.BytesValues[i]
		result.PutEmptyBytes(key).FromRaw(value)
	}
	return result
}
