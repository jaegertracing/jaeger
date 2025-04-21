// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func FromDBModel(dbTrace Trace) ptrace.Traces {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()

	resourceAttributes, err := AttributesGroupToMap(dbTrace.Resource.Attributes)
	if err != nil {
		jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes value: %v", err))
	}
	resourceAttributes.CopyTo(resourceSpans.Resource().Attributes())

	scope := scopeSpans.Scope()
	sc, err := FromDBScope(dbTrace.Scope)
	if err != nil {
		jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes value: %v", err))
	}
	sc.CopyTo(scope)

	sp, err := FromDBSpan(dbTrace.Span)
	if err != nil {
		jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes value: %v", err))
	}
	sp.CopyTo(span)
	for i := range dbTrace.Events {
		event := span.Events().AppendEmpty()
		e, err := FromDBEvent(dbTrace.Events[i])
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes value: %v", err))
		}
		e.CopyTo(event)
	}

	for i := range dbTrace.Links {
		link := span.Links().AppendEmpty()
		l, err := FromDBLink(dbTrace.Links[i])
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes value: %v", err))
		}
		l.CopyTo(link)
	}
	return trace
}

func FromDBScope(s Scope) (pcommon.InstrumentationScope, error) {
	scope := ptrace.NewScopeSpans().Scope()
	scope.SetName(s.Name)
	scope.SetVersion(s.Version)
	attributes, err := AttributesGroupToMap(s.Attributes)
	attributes.CopyTo(scope.Attributes())

	return scope, err
}

func FromDBSpan(s Span) (ptrace.Span, error) {
	span := ptrace.NewSpan()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(s.Timestamp))
	traceId, err := hex.DecodeString(s.TraceId)
	if err != nil {
		panic(err)
	}
	span.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(s.SpanId)
	if err != nil {
		panic(err)
	}
	span.SetSpanID(pcommon.SpanID(spanId))
	parentSpanId, err := hex.DecodeString(s.ParentSpanId)
	if err != nil {
		panic(err)
	}
	span.SetParentSpanID(pcommon.SpanID(parentSpanId))
	span.TraceState().FromRaw(s.TraceState)
	span.SetName(s.Name)
	span.SetKind(FromDBSpanKind(s.Kind))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(s.Duration))
	span.Status().SetCode(FromDBStatusCode(s.StatusCode))
	span.Status().SetMessage(s.StatusMessage)
	spanAttributes, err := AttributesGroupToMap(s.Attributes)
	spanAttributes.CopyTo(span.Attributes())

	return span, err
}

func FromDBEvent(e Event) (ptrace.SpanEvent, error) {
	event := ptrace.NewSpanEvent()
	event.SetName(e.Name)
	event.SetTimestamp(pcommon.NewTimestampFromTime(e.Timestamp))

	attributes, err := AttributesGroupToMap(e.Attributes)
	attributes.CopyTo(event.Attributes())
	return event, err
}

func FromDBLink(l Link) (ptrace.SpanLink, error) {
	link := ptrace.NewSpanLink()
	traceId, err := hex.DecodeString(l.TraceId)
	if err != nil {
		panic(err)
	}
	link.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(l.SpanId)
	if err != nil {
		panic(err)
	}
	link.SetSpanID(pcommon.SpanID(spanId))
	link.TraceState().FromRaw(l.TraceState)
	attributes, err := AttributesGroupToMap(l.Attributes)
	attributes.CopyTo(link.Attributes())
	return link, err
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

func AttributesGroupToMap(group AttributesGroup) (pcommon.Map, error) {
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
		bts, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return result, err
		}
		result.PutEmptyBytes(key).FromRaw(bts)
	}
	return result, nil
}
