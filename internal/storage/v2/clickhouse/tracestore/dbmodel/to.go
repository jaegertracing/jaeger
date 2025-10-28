// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"go.opentelemetry.io/collector/pdata/xpdata"
)

// ToRow converts an OpenTelemetry Span along with its Resource and Scope to a
// span row that can be stored in ClickHouse.
func ToRow(
	resource pcommon.Resource,
	scope pcommon.InstrumentationScope,
	span ptrace.Span,
) *SpanRow {
	// we assume a sanitizer was applied upstream to guarantee non-empty service name
	serviceName, _ := resource.Attributes().Get(otelsemconv.ServiceNameKey)
	duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Nanoseconds()
	sr := &SpanRow{
		ID:            span.SpanID().String(),
		TraceID:       span.TraceID().String(),
		TraceState:    span.TraceState().AsRaw(),
		ParentSpanID:  span.ParentSpanID().String(),
		Name:          span.Name(),
		Kind:          jptrace.SpanKindToString(span.Kind()),
		StartTime:     span.StartTimestamp().AsTime(),
		StatusCode:    span.Status().Code().String(),
		StatusMessage: span.Status().Message(),
		Duration:      duration,
		ServiceName:   serviceName.Str(),
		ScopeName:     scope.Name(),
		ScopeVersion:  scope.Version(),
	}
	appendAttributes(&sr.Attributes, span.Attributes())
	for _, event := range span.Events().All() {
		sr.appendEvent(event)
	}
	for _, link := range span.Links().All() {
		sr.appendLink(link)
	}
	appendAttributes(&sr.ResourceAttributes, resource.Attributes())
	appendAttributes(&sr.ScopeAttributes, scope.Attributes())

	return sr
}

func appendAttributes(dest *Attributes, attrs pcommon.Map) {
	a := extractAttributes(attrs)
	dest.BoolKeys = append(dest.BoolKeys, a.BoolKeys...)
	dest.BoolValues = append(dest.BoolValues, a.BoolValues...)
	dest.DoubleKeys = append(dest.DoubleKeys, a.DoubleKeys...)
	dest.DoubleValues = append(dest.DoubleValues, a.DoubleValues...)
	dest.IntKeys = append(dest.IntKeys, a.IntKeys...)
	dest.IntValues = append(dest.IntValues, a.IntValues...)
	dest.StrKeys = append(dest.StrKeys, a.StrKeys...)
	dest.StrValues = append(dest.StrValues, a.StrValues...)
	dest.ComplexKeys = append(dest.ComplexKeys, a.ComplexKeys...)
	dest.ComplexValues = append(dest.ComplexValues, a.ComplexValues...)
}

func appendAttributes2D(dest *Attributes2D, attrs pcommon.Map) {
	a := extractAttributes(attrs)
	dest.BoolKeys = append(dest.BoolKeys, a.BoolKeys)
	dest.BoolValues = append(dest.BoolValues, a.BoolValues)
	dest.DoubleKeys = append(dest.DoubleKeys, a.DoubleKeys)
	dest.DoubleValues = append(dest.DoubleValues, a.DoubleValues)
	dest.IntKeys = append(dest.IntKeys, a.IntKeys)
	dest.IntValues = append(dest.IntValues, a.IntValues)
	dest.StrKeys = append(dest.StrKeys, a.StrKeys)
	dest.StrValues = append(dest.StrValues, a.StrValues)
	dest.ComplexKeys = append(dest.ComplexKeys, a.ComplexKeys)
	dest.ComplexValues = append(dest.ComplexValues, a.ComplexValues)
}

func (sr *SpanRow) appendEvent(event ptrace.SpanEvent) {
	sr.EventNames = append(sr.EventNames, event.Name())
	sr.EventTimestamps = append(sr.EventTimestamps, event.Timestamp().AsTime())
	appendAttributes2D(&sr.EventAttributes, event.Attributes())
}

func (sr *SpanRow) appendLink(link ptrace.SpanLink) {
	sr.LinkTraceIDs = append(sr.LinkTraceIDs, link.TraceID().String())
	sr.LinkSpanIDs = append(sr.LinkSpanIDs, link.SpanID().String())
	sr.LinkTraceStates = append(sr.LinkTraceStates, link.TraceState().AsRaw())
	appendAttributes2D(&sr.LinkAttributes, link.Attributes())
}

func extractAttributes(attrs pcommon.Map) *Attributes {
	out := &Attributes{}
	attrs.Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeBool:
			out.BoolKeys = append(out.BoolKeys, k)
			out.BoolValues = append(out.BoolValues, v.Bool())
		case pcommon.ValueTypeDouble:
			out.DoubleKeys = append(out.DoubleKeys, k)
			out.DoubleValues = append(out.DoubleValues, v.Double())
		case pcommon.ValueTypeInt:
			out.IntKeys = append(out.IntKeys, k)
			out.IntValues = append(out.IntValues, v.Int())
		case pcommon.ValueTypeStr:
			out.StrKeys = append(out.StrKeys, k)
			out.StrValues = append(out.StrValues, v.Str())
		case pcommon.ValueTypeBytes:
			key := "@bytes@" + k
			encoded := base64.StdEncoding.EncodeToString(v.Bytes().AsRaw())
			out.ComplexKeys = append(out.ComplexKeys, key)
			out.ComplexValues = append(out.ComplexValues, encoded)
		case pcommon.ValueTypeSlice:
			key := "@slice@" + k
			m := &xpdata.JSONMarshaler{}
			b, err := m.MarshalValue(v)
			if err != nil {
				break
			}
			encoded := base64.StdEncoding.EncodeToString(b)
			out.ComplexKeys = append(out.ComplexKeys, key)
			out.ComplexValues = append(out.ComplexValues, encoded)
		case pcommon.ValueTypeMap:
			key := "@array@" + k
			m := &xpdata.JSONMarshaler{}
			b, err := m.MarshalValue(v)
			if err != nil {
				break
			}
			encoded := base64.StdEncoding.EncodeToString(b)
			out.ComplexKeys = append(out.ComplexKeys, key)
			out.ComplexValues = append(out.ComplexValues, encoded)
		default:
		}
		return true
	})
	return out
}
