// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"encoding/hex"
	"time"

	"github.com/ClickHouse/ch-go/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.27.0"
)

// Input converts the OTEL Traces model into the ClickHouse table format for batch writing.
func Input(td ptrace.Traces) proto.Input {
	var (
		timestamp                = new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano)
		traceId                  proto.ColStr
		spanId                   proto.ColStr
		parentSpanId             proto.ColStr
		traceState               proto.ColStr
		spanName                 proto.ColStr
		spanKind                 proto.ColStr
		serviceName              proto.ColStr
		resourceAttributesKeys   = new(proto.ColStr).LowCardinality().Array()
		resourceAttributesValues = new(proto.ColStr).LowCardinality().Array()
		scopeName                proto.ColStr
		scopeVersion             proto.ColStr
		spanAttributesKeys       = new(proto.ColStr).LowCardinality().Array()
		spanAttributesValues     = new(proto.ColStr).LowCardinality().Array()
		duration                 proto.ColUInt64
		statusCode               proto.ColStr
		statusMessage            proto.ColStr
		eventsTimestamp          = new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano).Array()
		eventsName               = new(proto.ColStr).Array()
		eventsAttributes         = proto.NewArray(proto.NewMap(new(proto.ColStr).LowCardinality(), new(proto.ColStr)))
		linksTraceId             = new(proto.ColStr).LowCardinality().Array()
		linksSpanId              = new(proto.ColStr).LowCardinality().Array()
		linksTraceState          = new(proto.ColStr).Array()
		linksAttributes          = proto.NewArray(proto.NewMap(new(proto.ColStr).LowCardinality(), new(proto.ColStr)))
	)

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		spans := td.ResourceSpans().At(i)
		res := spans.Resource()
		servName := getServiceName(res.Attributes())

		resourceAttrs := attributesToMap(res.Attributes())
		resourceKeys := make([]string, len(resourceAttrs))
		resourceValues := make([]string, len(resourceAttrs))
		idx := 0
		for key, val := range resourceAttrs {
			resourceKeys[idx] = key
			resourceValues[idx] = val
			idx++
		}

		for j := 0; j < spans.ScopeSpans().Len(); j++ {
			scope := spans.ScopeSpans().At(j).Scope()
			scName := scope.Name()
			scVersion := scope.Version()

			rs := spans.ScopeSpans().At(j).Spans()
			for k := 0; k < rs.Len(); k++ {
				r := rs.At(k)

				spanAttr := attributesToMap(r.Attributes())
				spanKeys := make([]string, len(spanAttr))
				spanValues := make([]string, len(spanAttr))
				idx = 0
				for key, val := range spanAttr {
					spanKeys[idx] = key
					spanValues[idx] = val
					idx++
				}

				status := r.Status()
				eventTimes, eventNames, eventAttrs := convertEvents(r.Events())
				link := convertLinks(r.Links())

				linksTraceStates := link.traceStates
				linksAttrs := link.attrs
				linksTraceIds := link.traceIDs
				linksSpanIds := link.spanIDs

				timestamp.Append(r.StartTimestamp().AsTime())
				traceId.Append(traceIDToHexOrEmptyString(r.TraceID()))
				spanId.Append(spanIDToHexOrEmptyString(r.SpanID()))
				parentSpanId.Append(spanIDToHexOrEmptyString(r.ParentSpanID()))
				traceState.Append(r.TraceState().AsRaw())
				spanName.Append(r.Name())
				spanKind.Append(r.Kind().String())
				serviceName.Append(servName)

				resourceAttributesKeys.Append(resourceKeys)
				resourceAttributesValues.Append(resourceValues)

				scopeName.Append(scName)
				scopeVersion.Append(scVersion)

				spanAttributesKeys.Append(spanKeys)
				spanAttributesValues.Append(spanValues)
				//nolint: gosec // G115
				duration.Append(uint64(r.EndTimestamp().AsTime().Sub(r.StartTimestamp().AsTime()).Nanoseconds()))
				statusCode.Append(status.Code().String())
				statusMessage.Append(status.Message())

				eventsTimestamp.Append(eventTimes)
				eventsName.Append(eventNames)
				eventsAttributes.Append(eventAttrs)

				linksTraceId.Append(linksTraceIds)
				linksSpanId.Append(linksSpanIds)
				linksTraceState.Append(linksTraceStates)
				linksAttributes.Append(linksAttrs)
			}
		}
	}

	return proto.Input{
		{Name: "Timestamp", Data: timestamp},
		{Name: "TraceId", Data: traceId},
		{Name: "SpanId", Data: spanId},
		{Name: "ParentSpanId", Data: parentSpanId},
		{Name: "TraceState", Data: traceState},
		{Name: "SpanName", Data: spanName},
		{Name: "SpanKind", Data: spanKind},
		{Name: "ServiceName", Data: serviceName},
		{Name: "ResourceAttributes.keys", Data: resourceAttributesKeys},
		{Name: "ResourceAttributes.values", Data: resourceAttributesValues},
		{Name: "ScopeName", Data: scopeName},
		{Name: "ScopeVersion", Data: scopeVersion},
		{Name: "SpanAttributes.keys", Data: spanAttributesKeys},
		{Name: "SpanAttributes.values", Data: spanAttributesValues},
		{Name: "Duration", Data: duration},
		{Name: "StatusCode", Data: statusCode},
		{Name: "StatusMessage", Data: statusMessage},
		{Name: "Events.Timestamp", Data: eventsTimestamp},
		{Name: "Events.Name", Data: eventsName},
		{Name: "Events.Attributes", Data: eventsAttributes},
		{Name: "Links.TraceId", Data: linksTraceId},
		{Name: "Links.SpanId", Data: linksSpanId},
		{Name: "Links.TraceState", Data: linksTraceState},
		{Name: "Links.Attributes", Data: linksAttributes},
	}
}

func attributesToMap(attributes pcommon.Map) map[string]string {
	result := map[string]string{}
	attributes.Range(func(k string, v pcommon.Value) bool {
		result[k] = v.AsString()
		return true
	})
	return result
}

func getServiceName(resAttr pcommon.Map) string {
	var serviceName string
	if v, ok := resAttr.Get(conventions.AttributeServiceName); ok {
		serviceName = v.AsString()
	}

	return serviceName
}

func convertEvents(events ptrace.SpanEventSlice) (times []time.Time, names []string, attrs []map[string]string) {
	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		times = append(times, event.Timestamp().AsTime())
		names = append(names, event.Name())
		attrs = append(attrs, attributesToMap(event.Attributes()))
	}
	return times, names, attrs
}

func convertLinks(links ptrace.SpanLinkSlice) link {
	traceIDs := make([]string, 0, links.Len())
	spanIDs := make([]string, 0, links.Len())
	states := make([]string, 0, links.Len())
	attrs := make([]map[string]string, 0, links.Len())

	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		traceIDs = append(traceIDs, traceIDToHexOrEmptyString(link.TraceID()))
		spanIDs = append(spanIDs, spanIDToHexOrEmptyString(link.SpanID()))
		states = append(states, link.TraceState().AsRaw())
		attrs = append(attrs, attributesToMap(link.Attributes()))
	}
	return link{traceIDs, spanIDs, states, attrs}
}

type link struct {
	traceIDs    []string
	spanIDs     []string
	traceStates []string
	attrs       []map[string]string
}

func traceIDToHexOrEmptyString(id pcommon.TraceID) string {
	if id.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(id[:])
}

func spanIDToHexOrEmptyString(id pcommon.SpanID) string {
	if id.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(id[:])
}
