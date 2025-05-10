// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// FromDBModel convert Clickhouse domain model to OTLP Trace model.
func FromDBModel(dbTrace Trace) ptrace.Traces {
	trace := ptrace.NewTraces()

	resourceSpans := trace.ResourceSpans().AppendEmpty()
	resource := resourceSpans.Resource()
	rs := FromDBResource(dbTrace.Resource)
	rs.CopyTo(resource)

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	scope := scopeSpans.Scope()
	sc := FromDBScope(dbTrace.Scope)
	sc.CopyTo(scope)

	span := scopeSpans.Spans().AppendEmpty()
	sp := FromDBSpan(dbTrace.Span)
	sp.CopyTo(span)

	for i := range dbTrace.Events {
		event := span.Events().AppendEmpty()
		e := FromDBEvent(dbTrace.Events[i])
		e.CopyTo(event)
	}

	for i := range dbTrace.Links {
		link := span.Links().AppendEmpty()
		l := FromDBLink(dbTrace.Links[i])
		l.CopyTo(link)
	}
	return trace
}

func FromDBResource(r Resource) pcommon.Resource {
	resource := ptrace.NewResourceSpans().Resource()
	resourceAttributes := AttributesGroupToMap(r.Attributes)
	resourceAttributes.CopyTo(resource.Attributes())
	return resource
}

func FromDBScope(s Scope) pcommon.InstrumentationScope {
	scope := ptrace.NewScopeSpans().Scope()
	scope.SetName(s.Name)
	scope.SetVersion(s.Version)
	attributes := AttributesGroupToMap(s.Attributes)
	attributes.CopyTo(scope.Attributes())
	return scope
}

func FromDBSpan(s Span) ptrace.Span {
	span := ptrace.NewSpan()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(s.Timestamp))
	span.SetTraceID(pcommon.TraceID(s.TraceId))
	span.SetSpanID(pcommon.SpanID(s.SpanId))
	span.SetParentSpanID(pcommon.SpanID(s.ParentSpanId))
	span.TraceState().FromRaw(s.TraceState)
	span.SetName(s.Name)
	span.SetKind(FromDBSpanKind(s.Kind))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(s.Duration))
	span.Status().SetCode(FromDBStatusCode(s.StatusCode))
	span.Status().SetMessage(s.StatusMessage)
	spanAttributes := AttributesGroupToMap(s.Attributes)
	spanAttributes.CopyTo(span.Attributes())
	return span
}

func FromDBEvent(e Event) ptrace.SpanEvent {
	event := ptrace.NewSpanEvent()
	event.SetName(e.Name)
	event.SetTimestamp(pcommon.NewTimestampFromTime(e.Timestamp))
	attributes := AttributesGroupToMap(e.Attributes)
	attributes.CopyTo(event.Attributes())
	return event
}

func FromDBLink(l Link) ptrace.SpanLink {
	link := ptrace.NewSpanLink()
	link.SetTraceID(pcommon.TraceID(l.TraceId))
	link.SetSpanID(pcommon.SpanID(l.SpanId))
	link.TraceState().FromRaw(l.TraceState)
	attributes := AttributesGroupToMap(l.Attributes)
	attributes.CopyTo(link.Attributes())
	return link
}

func FromDBSpanKind(sk string) ptrace.SpanKind {
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

func FromDBStatusCode(sc string) ptrace.StatusCode {
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

func AttributesGroupToMap(group AttributesGroup) pcommon.Map {
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
