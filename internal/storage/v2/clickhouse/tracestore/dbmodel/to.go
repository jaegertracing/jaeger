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
	dest.BoolKeys = append(dest.BoolKeys, a.boolKeys...)
	dest.BoolValues = append(dest.BoolValues, a.boolValues...)
	dest.DoubleKeys = append(dest.DoubleKeys, a.doubleKeys...)
	dest.DoubleValues = append(dest.DoubleValues, a.doubleValues...)
	dest.IntKeys = append(dest.IntKeys, a.intKeys...)
	dest.IntValues = append(dest.IntValues, a.intValues...)
	dest.StrKeys = append(dest.StrKeys, a.strKeys...)
	dest.StrValues = append(dest.StrValues, a.strValues...)
	dest.ComplexKeys = append(dest.ComplexKeys, a.complexKeys...)
	dest.ComplexValues = append(dest.ComplexValues, a.complexValues...)
}

func appendAttributes2D(dest *Attributes2D, attrs pcommon.Map) {
	a := extractAttributes(attrs)
	dest.BoolKeys = append(dest.BoolKeys, a.boolKeys)
	dest.BoolValues = append(dest.BoolValues, a.boolValues)
	dest.DoubleKeys = append(dest.DoubleKeys, a.doubleKeys)
	dest.DoubleValues = append(dest.DoubleValues, a.doubleValues)
	dest.IntKeys = append(dest.IntKeys, a.intKeys)
	dest.IntValues = append(dest.IntValues, a.intValues)
	dest.StrKeys = append(dest.StrKeys, a.strKeys)
	dest.StrValues = append(dest.StrValues, a.strValues)
	dest.ComplexKeys = append(dest.ComplexKeys, a.complexKeys)
	dest.ComplexValues = append(dest.ComplexValues, a.complexValues)
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

func extractAttributes(attrs pcommon.Map) (out struct {
	boolKeys      []string
	boolValues    []bool
	doubleKeys    []string
	doubleValues  []float64
	intKeys       []string
	intValues     []int64
	strKeys       []string
	strValues     []string
	complexKeys   []string
	complexValues []string
},
) {
	attrs.Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeBool:
			out.boolKeys = append(out.boolKeys, k)
			out.boolValues = append(out.boolValues, v.Bool())
		case pcommon.ValueTypeDouble:
			out.doubleKeys = append(out.doubleKeys, k)
			out.doubleValues = append(out.doubleValues, v.Double())
		case pcommon.ValueTypeInt:
			out.intKeys = append(out.intKeys, k)
			out.intValues = append(out.intValues, v.Int())
		case pcommon.ValueTypeStr:
			out.strKeys = append(out.strKeys, k)
			out.strValues = append(out.strValues, v.Str())
		case pcommon.ValueTypeBytes:
			key := "@bytes@" + k
			encoded := base64.StdEncoding.EncodeToString(v.Bytes().AsRaw())
			out.complexKeys = append(out.complexKeys, key)
			out.complexValues = append(out.complexValues, encoded)
		case pcommon.ValueTypeSlice:
			key := "@slice@" + k
			m := &xpdata.JSONMarshaler{}
			b, err := m.MarshalValue(v)
			if err != nil {
				break
			}
			encoded := base64.StdEncoding.EncodeToString(b)
			out.complexKeys = append(out.complexKeys, key)
			out.complexValues = append(out.complexValues, encoded)
		case pcommon.ValueTypeMap:
			key := "@array@" + k
			m := &xpdata.JSONMarshaler{}
			b, err := m.MarshalValue(v)
			if err != nil {
				break
			}
			encoded := base64.StdEncoding.EncodeToString(b)
			out.complexKeys = append(out.complexKeys, key)
			out.complexValues = append(out.complexValues, encoded)
		default:
		}
		return true
	})
	return out
}
