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
		RawDuration:   duration,
		ServiceName:   serviceName.Str(),
		ScopeName:     scope.Name(),
		ScopeVersion:  scope.Version(),
	}
	sr.appendSpanAttributes(span.Attributes())
	for _, event := range span.Events().All() {
		sr.appendEvent(event)
	}
	for _, link := range span.Links().All() {
		sr.appendLink(link)
	}

	return sr
}

func (sr *SpanRow) appendSpanAttributes(attrs pcommon.Map) {
	a := extractAttributes(attrs)
	sr.BoolAttributeKeys = append(sr.BoolAttributeKeys, a.boolKeys...)
	sr.BoolAttributeValues = append(sr.BoolAttributeValues, a.boolValues...)
	sr.DoubleAttributeKeys = append(sr.DoubleAttributeKeys, a.doubleKeys...)
	sr.DoubleAttributeValues = append(sr.DoubleAttributeValues, a.doubleValues...)
	sr.IntAttributeKeys = append(sr.IntAttributeKeys, a.intKeys...)
	sr.IntAttributeValues = append(sr.IntAttributeValues, a.intValues...)
	sr.StrAttributeKeys = append(sr.StrAttributeKeys, a.strKeys...)
	sr.StrAttributeValues = append(sr.StrAttributeValues, a.strValues...)
	sr.ComplexAttributeKeys = append(sr.ComplexAttributeKeys, a.complexKeys...)
	sr.ComplexAttributeValues = append(sr.ComplexAttributeValues, a.complexValues...)
}

func (sr *SpanRow) appendEvent(event ptrace.SpanEvent) {
	sr.EventNames = append(sr.EventNames, event.Name())
	sr.EventTimestamps = append(sr.EventTimestamps, event.Timestamp().AsTime())

	evAttrs := extractAttributes(event.Attributes())
	sr.EventBoolAttributeKeys = append(sr.EventBoolAttributeKeys, evAttrs.boolKeys)
	sr.EventBoolAttributeValues = append(sr.EventBoolAttributeValues, evAttrs.boolValues)
	sr.EventDoubleAttributeKeys = append(sr.EventDoubleAttributeKeys, evAttrs.doubleKeys)
	sr.EventDoubleAttributeValues = append(sr.EventDoubleAttributeValues, evAttrs.doubleValues)
	sr.EventIntAttributeKeys = append(sr.EventIntAttributeKeys, evAttrs.intKeys)
	sr.EventIntAttributeValues = append(sr.EventIntAttributeValues, evAttrs.intValues)
	sr.EventStrAttributeKeys = append(sr.EventStrAttributeKeys, evAttrs.strKeys)
	sr.EventStrAttributeValues = append(sr.EventStrAttributeValues, evAttrs.strValues)
	sr.EventComplexAttributeKeys = append(sr.EventComplexAttributeKeys, evAttrs.complexKeys)
	sr.EventComplexAttributeValues = append(sr.EventComplexAttributeValues, evAttrs.complexValues)
}

func (sr *SpanRow) appendLink(link ptrace.SpanLink) {
	sr.LinkTraceIDs = append(sr.LinkTraceIDs, link.TraceID().String())
	sr.LinkSpanIDs = append(sr.LinkSpanIDs, link.SpanID().String())
	sr.LinkTraceStates = append(sr.LinkTraceStates, link.TraceState().AsRaw())

	linkAttrs := extractAttributes(link.Attributes())
	sr.LinkBoolAttributeKeys = append(sr.LinkBoolAttributeKeys, linkAttrs.boolKeys)
	sr.LinkBoolAttributeValues = append(sr.LinkBoolAttributeValues, linkAttrs.boolValues)
	sr.LinkDoubleAttributeKeys = append(sr.LinkDoubleAttributeKeys, linkAttrs.doubleKeys)
	sr.LinkDoubleAttributeValues = append(sr.LinkDoubleAttributeValues, linkAttrs.doubleValues)
	sr.LinkIntAttributeKeys = append(sr.LinkIntAttributeKeys, linkAttrs.intKeys)
	sr.LinkIntAttributeValues = append(sr.LinkIntAttributeValues, linkAttrs.intValues)
	sr.LinkStrAttributeKeys = append(sr.LinkStrAttributeKeys, linkAttrs.strKeys)
	sr.LinkStrAttributeValues = append(sr.LinkStrAttributeValues, linkAttrs.strValues)
	sr.LinkComplexAttributeKeys = append(sr.LinkComplexAttributeKeys, linkAttrs.complexKeys)
	sr.LinkComplexAttributeValues = append(sr.LinkComplexAttributeValues, linkAttrs.complexValues)
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
		//revive:disable
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
		case pcommon.ValueTypeSlice, pcommon.ValueTypeMap:
			// TODO
		default:
			//revive:enable
		}
		return true
	})
	return out
}
