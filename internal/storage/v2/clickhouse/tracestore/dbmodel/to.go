// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/xpdata"

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
	appendAttributes(&sr.ScopeAttributes, scope.Attributes())

	return sr
}

func appendAttributes(dest *Attributes, attrs pcommon.Map) {
	a := extractAttributes(attrs)
	dest.Keys = append(dest.Keys, a.Keys...)
	dest.Values = append(dest.Values, a.Values...)
	dest.Types = append(dest.Types, a.Types...)
}

func appendAttributes2D(dest *Attributes2D, attrs pcommon.Map) {
	a := extractAttributes(attrs)
	dest.Keys = append(dest.Keys, a.Keys)
	dest.Values = append(dest.Values, a.Values)
	dest.Types = append(dest.Types, a.Types)
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
		case pcommon.ValueTypeBool, pcommon.ValueTypeDouble, pcommon.ValueTypeInt, pcommon.ValueTypeStr:
			out.Keys = append(out.Keys, k)
			out.Values = append(out.Values, v.AsString())
			out.Types = append(out.Types, v.Type().String())
		case pcommon.ValueTypeBytes:
			encoded := base64.StdEncoding.EncodeToString(v.Bytes().AsRaw())
			out.Keys = append(out.Keys, k)
			out.Values = append(out.Values, encoded)
			out.Types = append(out.Types, v.Type().String())
		case pcommon.ValueTypeSlice:
			m := &xpdata.JSONMarshaler{}
			b, err := m.MarshalValue(v)
			if err != nil {
				out.Keys = append(out.Keys, jptrace.WarningsAttribute)
				out.Values = append(
					out.Values,
					fmt.Sprintf("failed to marshal slice attribute %q: %v", k, err))
				out.Types = append(out.Types, pcommon.ValueTypeStr.String())
				break
			}
			out.Keys = append(out.Keys, k)
			out.Values = append(out.Values, string(b))
			out.Types = append(out.Types, v.Type().String())
		case pcommon.ValueTypeMap:
			m := &xpdata.JSONMarshaler{}
			b, err := m.MarshalValue(v)
			if err != nil {
				out.Keys = append(out.Keys, jptrace.WarningsAttribute)
				out.Values = append(
					out.Values,
					fmt.Sprintf("failed to marshal map attribute %q: %v", k, err))
				out.Types = append(out.Types, pcommon.ValueTypeStr.String())
				break
			}
			out.Keys = append(out.Keys, k)
			out.Values = append(out.Values, string(b))
			out.Types = append(out.Types, v.Type().String())
		default:
		}
		return true
	})
	return out
}
