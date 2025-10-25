// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
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

func (sr *SpanRow) appendEvent(event ptrace.SpanEvent) {
	sr.EventNames = append(sr.EventNames, event.Name())
	sr.EventTimestamps = append(sr.EventTimestamps, event.Timestamp().AsTime())

	evAttrs := extractAttributes(event.Attributes())
	sr.EventAttributes.BoolKeys = append(sr.EventAttributes.BoolKeys, evAttrs.boolKeys)
	sr.EventAttributes.BoolValues = append(sr.EventAttributes.BoolValues, evAttrs.boolValues)
	sr.EventAttributes.DoubleKeys = append(sr.EventAttributes.DoubleKeys, evAttrs.doubleKeys)
	sr.EventAttributes.DoubleValues = append(sr.EventAttributes.DoubleValues, evAttrs.doubleValues)
	sr.EventAttributes.IntKeys = append(sr.EventAttributes.IntKeys, evAttrs.intKeys)
	sr.EventAttributes.IntValues = append(sr.EventAttributes.IntValues, evAttrs.intValues)
	sr.EventAttributes.StrKeys = append(sr.EventAttributes.StrKeys, evAttrs.strKeys)
	sr.EventAttributes.StrValues = append(sr.EventAttributes.StrValues, evAttrs.strValues)
	sr.EventAttributes.ComplexKeys = append(sr.EventAttributes.ComplexKeys, evAttrs.complexKeys)
	sr.EventAttributes.ComplexValues = append(sr.EventAttributes.ComplexValues, evAttrs.complexValues)
}

func (sr *SpanRow) appendLink(link ptrace.SpanLink) {
	sr.LinkTraceIDs = append(sr.LinkTraceIDs, link.TraceID().String())
	sr.LinkSpanIDs = append(sr.LinkSpanIDs, link.SpanID().String())
	sr.LinkTraceStates = append(sr.LinkTraceStates, link.TraceState().AsRaw())

	linkAttrs := extractAttributes(link.Attributes())
	sr.LinkAttributes.BoolKeys = append(sr.LinkAttributes.BoolKeys, linkAttrs.boolKeys)
	sr.LinkAttributes.BoolValues = append(sr.LinkAttributes.BoolValues, linkAttrs.boolValues)
	sr.LinkAttributes.DoubleKeys = append(sr.LinkAttributes.DoubleKeys, linkAttrs.doubleKeys)
	sr.LinkAttributes.DoubleValues = append(sr.LinkAttributes.DoubleValues, linkAttrs.doubleValues)
	sr.LinkAttributes.IntKeys = append(sr.LinkAttributes.IntKeys, linkAttrs.intKeys)
	sr.LinkAttributes.IntValues = append(sr.LinkAttributes.IntValues, linkAttrs.intValues)
	sr.LinkAttributes.StrKeys = append(sr.LinkAttributes.StrKeys, linkAttrs.strKeys)
	sr.LinkAttributes.StrValues = append(sr.LinkAttributes.StrValues, linkAttrs.strValues)
	sr.LinkAttributes.ComplexKeys = append(sr.LinkAttributes.ComplexKeys, linkAttrs.complexKeys)
	sr.LinkAttributes.ComplexValues = append(sr.LinkAttributes.ComplexValues, linkAttrs.complexValues)
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
		// TODO: support array and map types
		default:
		}
		return true
	})
	return out
}
