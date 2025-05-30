// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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

	sp, err := FromDBSpan(dbTrace.Span)
	sp.CopyTo(span)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}

	resource := resourceSpans.Resource()
	rs, err := FromDBResource(dbTrace.Resource)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}
	rs.CopyTo(resource)

	scope := scopeSpans.Scope()
	sc, err := FromDBScope(dbTrace.Scope)
	if err != nil {
		jptrace.AddWarnings(span, err.Error())
	}
	sc.CopyTo(scope)

	for i := range dbTrace.Events {
		event := span.Events().AppendEmpty()
		e, err := FromDBEvent(dbTrace.Events[i])
		if err != nil {
			jptrace.AddWarnings(span, err.Error())
		}
		e.CopyTo(event)
	}

	for i := range dbTrace.Links {
		link := span.Links().AppendEmpty()
		l, err := FromDBLink(dbTrace.Links[i])
		if err != nil {
			jptrace.AddWarnings(span, err.Error())
		}
		l.CopyTo(link)
	}
	return trace
}

func FromDBResource(r Resource) (pcommon.Resource, error) {
	resource := ptrace.NewResourceSpans().Resource()
	resourceAttributes, err := AttributesGroupToMap(r.Attributes)
	if err != nil {
		return resource, fmt.Errorf("failed to decode resource attribute: %w", err)
	}
	resourceAttributes.CopyTo(resource.Attributes())
	return resource, nil
}

func FromDBScope(s Scope) (pcommon.InstrumentationScope, error) {
	scope := ptrace.NewScopeSpans().Scope()
	scope.SetName(s.Name)
	scope.SetVersion(s.Version)
	attributes, err := AttributesGroupToMap(s.Attributes)
	if err != nil {
		return scope, fmt.Errorf("failed to decode scope attribute: %w", err)
	}
	attributes.CopyTo(scope.Attributes())

	return scope, nil
}

func FromDBSpan(s Span) (ptrace.Span, error) {
	span := ptrace.NewSpan()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(s.Timestamp))
	traceId, err := hex.DecodeString(s.TraceId)
	if err != nil {
		return span, fmt.Errorf("failed to decode trace Id: %w", err)
	}
	span.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(s.SpanId)
	if err != nil {
		return span, fmt.Errorf("failed to decode span Id: %w", err)
	}
	span.SetSpanID(pcommon.SpanID(spanId))
	parentSpanId, err := hex.DecodeString(s.ParentSpanId)
	if err != nil {
		return span, fmt.Errorf("failed to decode parent span Id: %w", err)
	}
	span.SetParentSpanID(pcommon.SpanID(parentSpanId))
	span.TraceState().FromRaw(s.TraceState)
	span.SetName(s.Name)
	span.SetKind(FromDBSpanKind(s.Kind))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(s.Duration))
	span.Status().SetCode(FromDBStatusCode(s.StatusCode))
	span.Status().SetMessage(s.StatusMessage)
	spanAttributes, err := AttributesGroupToMap(s.Attributes)
	if err != nil {
		return span, fmt.Errorf("failed to decode span attribute: %w", err)
	}
	spanAttributes.CopyTo(span.Attributes())

	return span, nil
}

func FromDBEvent(e Event) (ptrace.SpanEvent, error) {
	event := ptrace.NewSpanEvent()
	event.SetName(e.Name)
	event.SetTimestamp(pcommon.NewTimestampFromTime(e.Timestamp))

	attributes, err := AttributesGroupToMap(e.Attributes)
	if err != nil {
		return event, fmt.Errorf("failed to decode event attribute: %w", err)
	}
	attributes.CopyTo(event.Attributes())
	return event, nil
}

func FromDBLink(l Link) (ptrace.SpanLink, error) {
	link := ptrace.NewSpanLink()
	traceId, err := hex.DecodeString(l.TraceId)
	if err != nil {
		return link, fmt.Errorf("failed to decode link trace Id: %w", err)
	}
	link.SetTraceID(pcommon.TraceID(traceId))
	spanId, err := hex.DecodeString(l.SpanId)
	if err != nil {
		return link, fmt.Errorf("failed to decode link span Id: %w", err)
	}
	link.SetSpanID(pcommon.SpanID(spanId))
	link.TraceState().FromRaw(l.TraceState)
	attributes, err := AttributesGroupToMap(l.Attributes)
	if err != nil {
		return link, fmt.Errorf("failed to decode link attribute: %w", err)
	}
	attributes.CopyTo(link.Attributes())
	return link, nil
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

		var parsed interface{}
		decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
		decoder.UseNumber()
		
		if err := decoder.Decode(&parsed); err == nil {
			switch v := parsed.(type) {
			case map[string]interface{}:
				m := result.PutEmptyMap(key)
				if err := interfaceToPcommonMap(v, m); err != nil {
					return result, err
				}
			case []interface{}:
				s := result.PutEmptySlice(key)
				if err := interfaceToPcommonSlice(v, s); err != nil {
					return result, err
				}
			default:
				result.PutStr(key, value)
			}
		} else {
			result.PutStr(key, value)
		}
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

func interfaceToPcommonMap(data map[string]interface{}, m pcommon.Map) error {
	for k, v := range data {
		val := m.PutEmpty(k)
		if err := interfaceToPcommonValue(v, val); err != nil {
			return err
		}
	}
	return nil
}

func interfaceToPcommonSlice(data []interface{}, s pcommon.Slice) error {
	s.EnsureCapacity(len(data))
	for _, item := range data {
		val := s.AppendEmpty()
		if err := interfaceToPcommonValue(item, val); err != nil {
			return err
		}
	}
	return nil
}

func interfaceToPcommonValue(data interface{}, val pcommon.Value) error {
	switch v := data.(type) {
	case bool:
		val.SetBool(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			val.SetInt(i)
		} else if f, err := v.Float64(); err == nil {
			val.SetDouble(f)
		}
	case float64:
		val.SetDouble(v)
	case string:
		val.SetStr(v)
	case []byte:
		val.SetEmptyBytes().FromRaw(v)
	case map[string]interface{}:
		m := val.SetEmptyMap()
		return interfaceToPcommonMap(v, m)
	case []interface{}:
		s := val.SetEmptySlice()
		return interfaceToPcommonSlice(v, s)
	}
	return nil
}
