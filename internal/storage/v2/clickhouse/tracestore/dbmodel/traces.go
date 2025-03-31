// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/hex"

	"github.com/ClickHouse/ch-go/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.27.0"
)

var (
	timestamp                        = new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano)
	traceId                          proto.ColStr
	spanId                           proto.ColStr
	parentSpanId                     proto.ColStr
	traceState                       proto.ColStr
	spanName                         proto.ColStr
	spanKind                         proto.ColStr
	serviceName                      proto.ColStr
	resourceAttributesBoolKeys       = new(proto.ColStr).Array()
	resourceAttributesBoolValues     = new(proto.ColBool).Array()
	resourceAttributesDoubleKeys     = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesDoubleValues   = new(proto.ColFloat64).LowCardinality().Array()
	resourceAttributesIntKeys        = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesIntValues      = new(proto.ColInt64).LowCardinality().Array()
	resourceAttributesStrKeys        = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesStrValues      = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesEmptyKeys      = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesEmptyMapKeys   = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesEmptySliceKeys = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesEmptyBytesKeys = new(proto.ColStr).LowCardinality().Array()
	scopeName                        proto.ColStr
	scopeVersion                     proto.ColStr
	scopeAttributesBoolKeys          = new(proto.ColStr).Array()
	scopeAttributesBoolValues        = new(proto.ColBool).Array()
	scopeAttributesDoubleKeys        = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesDoubleValues      = new(proto.ColFloat64).LowCardinality().Array()
	scopeAttributesIntKeys           = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesIntValues         = new(proto.ColInt64).LowCardinality().Array()
	scopeAttributesStrKeys           = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesStrValues         = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesEmptyKeys         = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesEmptyMapKeys      = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesEmptySliceKeys    = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesEmptyBytesKeys    = new(proto.ColStr).LowCardinality().Array()
	spanAttributesBoolKeys           = new(proto.ColStr).Array()
	spanAttributesBoolValues         = new(proto.ColBool).Array()
	spanAttributesDoubleKeys         = new(proto.ColStr).LowCardinality().Array()
	spanAttributesDoubleValues       = new(proto.ColFloat64).LowCardinality().Array()
	spanAttributesIntKeys            = new(proto.ColStr).LowCardinality().Array()
	spanAttributesIntValues          = new(proto.ColInt64).LowCardinality().Array()
	spanAttributesStrKeys            = new(proto.ColStr).LowCardinality().Array()
	spanAttributesStrValues          = new(proto.ColStr).LowCardinality().Array()
	spanAttributesEmptyKeys          = new(proto.ColStr).LowCardinality().Array()
	spanAttributesEmptyMapKeys       = new(proto.ColStr).LowCardinality().Array()
	spanAttributesEmptySliceKeys     = new(proto.ColStr).LowCardinality().Array()
	spanAttributesEmptyBytesKeys     = new(proto.ColStr).LowCardinality().Array()
	duration                         = new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano)
	statusCode                       proto.ColStr
	statusMessage                    proto.ColStr
	eventsTimestamp                  = new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano).Array()
	eventsName                       = new(proto.ColStr).Array()
	eventsAttributesBoolKeys         = proto.NewArray(new(proto.ColStr).Array())
	eventsAttributesBoolValues       = proto.NewArray(new(proto.ColBool).Array())
	eventsAttributesDoubleKeys       = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesDoubleValues     = proto.NewArray(new(proto.ColFloat64).LowCardinality().Array())
	eventsAttributesIntKeys          = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesIntValues        = proto.NewArray(new(proto.ColInt64).LowCardinality().Array())
	eventsAttributesStrKeys          = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesStrValues        = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesEmptyKeys        = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesEmptyMapKeys     = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesEmptySlicesKeys  = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesEmptyBytesKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksTraceId                     = new(proto.ColStr).LowCardinality().Array()
	linksSpanId                      = new(proto.ColStr).LowCardinality().Array()
	linksTraceState                  = new(proto.ColStr).Array()
	linksAttributesBoolKeys          = proto.NewArray(new(proto.ColStr).Array())
	linksAttributesBoolValues        = proto.NewArray(new(proto.ColBool).Array())
	linksAttributesDoubleKeys        = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesDoubleValues      = proto.NewArray(new(proto.ColFloat64).LowCardinality().Array())
	linksAttributesIntKeys           = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesIntValues         = proto.NewArray(new(proto.ColInt64).LowCardinality().Array())
	linksAttributesStrKeys           = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesStrValues         = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesEmptyKeys         = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesEmptyMapKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesEmptySlicesKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesEmptyBytesKeys    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
)

type AttributeType int32

const (
	AttributeTypeResource AttributeType = iota
	AttributeTypeScope
	AttributeTypeSpan
	AttributeTypeEvent
	AttributeTypeLink
)

const (
	ColTimestamp                         = "timestamp"
	ColTraceId                           = "traceId"
	ColSpanId                            = "spanId"
	ColParentSpanId                      = "parentSpanId"
	ColTraceState                        = "traceState"
	ColSpanName                          = "spanName"
	ColSpanKind                          = "spanKind"
	ColServiceName                       = "serviceName"
	ColResourceAttributesBoolKeys        = "resourceAttributesBoolKeys"
	ColResourceAttributesBoolValues      = "resourceAttributesBoolValues"
	ColResourceAttributesDoubleKeys      = "resourceAttributesDoubleKeys"
	ColResourceAttributesDoubleValues    = "resourceAttributesDoubleValues"
	ColResourceAttributesIntKeys         = "resourceAttributesIntKeys"
	ColResourceAttributesIntValues       = "resourceAttributesIntValues"
	ColResourceAttributesStrKeys         = "resourceAttributesStrKeys"
	ColResourceAttributesStrValues       = "resourceAttributesStrValues"
	ColResourceAttributesEmptyKeys       = "resourceAttributesEmptyKeys"
	ColResourceAttributesEmptyMapKeys    = "resourceAttributesEmptyMapKeys"
	ColResourceAttributesEmptySlicesKeys = "resourceAttributesEmptySlicesKeys"
	ColResourceAttributesEmptyBytesKeys  = "resourceAttributesEmptyBytesKeys"
	ColScopeName                         = "scopeName"
	ColScopeVersion                      = "scopeVersion"
	ColScopeAttributesBoolKeys           = "scopeAttributesBoolKeys"
	ColScopeAttributesBoolValues         = "scopeAttributesBoolValues"
	ColScopeAttributesDoubleKeys         = "scopeAttributesDoubleKeys"
	ColScopeAttributesDoubleValues       = "scopeAttributesDoubleValues"
	ColScopeAttributesIntKeys            = "scopeAttributesIntKeys"
	ColScopeAttributesIntValues          = "scopeAttributesIntValues"
	ColScopeAttributesStrKeys            = "scopeAttributesStrKeys"
	ColScopeAttributesStrValues          = "scopeAttributesStrValues"
	ColScopeAttributesEmptyKeys          = "scopeAttributesEmptyKeys"
	ColScopeAttributesEmptyMapKeys       = "scopeAttributesEmptyMapKeys"
	ColScopeAttributesEmptySlicesKeys    = "scopeAttributesEmptySlicesKeys"
	ColScopeAttributesEmptyBytesKeys     = "scopeAttributesEmptyBytesKeys"
	ColSpanAttributesBoolKeys            = "spanAttributesBoolKeys"
	ColSpanAttributesBoolValues          = "spanAttributesBoolValues"
	ColSpanAttributesDoubleKeys          = "spanAttributesDoubleKeys"
	ColSpanAttributesDoubleValues        = "spanAttributesDoubleValues"
	ColSpanAttributesIntKeys             = "spanAttributesIntKeys"
	ColSpanAttributesIntValues           = "spanAttributesIntValues"
	ColSpanAttributesStrKeys             = "spanAttributesStrKeys"
	ColSpanAttributesStrValues           = "spanAttributesStrValues"
	ColSpanAttributesEmptyKeys           = "spanAttributesEmptyKeys"
	ColSpanAttributesEmptyMapKeys        = "spanAttributesEmptyMapKeys"
	ColSpanAttributesEmptySlicesKeys     = "spanAttributesEmptySlicesKeys"
	ColSpanAttributesEmptyBytesKeys      = "spanAttributesEmptyBytesKeys"
	ColDuration                          = "duration"
	ColStatusCode                        = "statusCode"
	ColStatusMessage                     = "statusMessage"
	ColEventsTimestamp                   = "eventsTimestamp"
	ColEventsName                        = "eventsName"
	ColEventsAttributesBoolKeys          = "eventsAttributesBoolKeys"
	ColEventsAttributesBoolValues        = "eventsAttributesBoolValues"
	ColEventsAttributesDoubleKeys        = "eventsAttributesDoubleKeys"
	ColEventsAttributesDoubleValues      = "eventsAttributesDoubleValues"
	ColEventsAttributesIntKeys           = "eventsAttributesIntKeys"
	ColEventsAttributesIntValues         = "eventsAttributesIntValues"
	ColEventsAttributesStrKeys           = "eventsAttributesStrKeys"
	ColEventsAttributesStrValues         = "eventsAttributesStrValues"
	ColEventsAttributesEmptyKeys         = "eventsAttributesEmptyKeys"
	ColEventsAttributesEmptyMapKeys      = "eventsAttributesEmptyMapKeys"
	ColEventsAttributesEmptySlicesKeys   = "eventsAttributesEmptySlicesKeys"
	ColEventsAttributesEmptyBytesKeys    = "eventsAttributesEmptyBytesKeys"
	ColLinksTraceId                      = "linksTraceId"
	ColLinksSpanId                       = "linksSpanId"
	ColLinksTraceState                   = "linksTraceState"
	ColLinksAttributesBoolKeys           = "linksAttributesBoolKeys"
	ColLinksAttributesBoolValues         = "linksAttributesBoolValues"
	ColLinksAttributesDoubleKeys         = "linksAttributesDoubleKeys"
	ColLinksAttributesDoubleValues       = "linksAttributesDoubleValues"
	ColLinksAttributesIntKeys            = "linksAttributesIntKeys"
	ColLinksAttributesIntValues          = "linksAttributesIntValues"
	ColLinksAttributesStrKeys            = "linksAttributesStrKeys"
	ColLinksAttributesStrValues          = "linksAttributesStrValues"
	ColLinksAttributesEmptyKeys          = "linksAttributesEmptyKeys"
	ColLinksAttributesEmptyMapKeys       = "linksAttributesEmptyMapKeys"
	ColLinksAttributesEmptySlicesKeys    = "linksAttributesEmptySlicesKeys"
	ColLinksAttributesEmptyBytesKeys     = "linksAttributesEmptyBytesKeys"
)

// FromPtrace converts the OpenTelemetry Traces model into a ClickHouse-compatible format for batch insertion.
// It maps the trace attributes, spans, links and events from the OTel model to the appropriate ClickHouse column types
func FromPtrace(td ptrace.Traces) proto.Input {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		spans := td.ResourceSpans().At(i)
		resource := spans.Resource()
		servName := getServiceName(resource.Attributes())
		resourceAttrs := toAttributesMap(resource.Attributes())

		for j := 0; j < spans.ScopeSpans().Len(); j++ {
			scope := spans.ScopeSpans().At(j).Scope()
			scopeAttrs := toAttributesMap(scope.Attributes())

			rs := spans.ScopeSpans().At(j).Spans()
			for k := 0; k < rs.Len(); k++ {
				r := rs.At(k)

				timestamp.Append(r.StartTimestamp().AsTime())
				traceId.Append(traceIDToHexOrEmptyString(r.TraceID()))
				spanId.Append(spanIDToHexOrEmptyString(r.SpanID()))
				parentSpanId.Append(spanIDToHexOrEmptyString(r.ParentSpanID()))
				traceState.Append(r.TraceState().AsRaw())
				serviceName.Append(servName)
				spanName.Append(r.Name())
				spanKind.Append(r.Kind().String())

				spansAttr := toAttributesMap(r.Attributes())
				resourceAG := toAttributesGroup(resourceAttrs)
				appendAttributes(AttributeTypeResource, resourceAG)
				scopeAG := toAttributesGroup(scopeAttrs)
				appendAttributes(AttributeTypeScope, scopeAG)
				spanAG := toAttributesGroup(spansAttr)
				appendAttributes(AttributeTypeSpan, spanAG)

				scName := scope.Name()
				scVersion := scope.Version()
				scopeName.Append(scName)
				scopeVersion.Append(scVersion)
				duration.Append(r.EndTimestamp().AsTime())

				status := r.Status()
				statusCode.Append(status.Code().String())
				statusMessage.Append(status.Message())

				events := fromEvents(r.Events())
				eventsTimestamp.Append(events.timestamps)
				eventsName.Append(events.names)
				appendNestedAttributes(AttributeTypeEvent, events.attributes)

				links := fromLinks(r.Links())
				linksTraceId.Append(links.traceIDs)
				linksSpanId.Append(links.spanIDs)
				linksTraceState.Append(links.traceStates)
				appendNestedAttributes(AttributeTypeLink, links.attributes)
			}
		}
	}

	return proto.Input{
		{Name: ColTimestamp, Data: timestamp},
		{Name: ColTraceId, Data: traceId},
		{Name: ColSpanId, Data: spanId},
		{Name: ColParentSpanId, Data: parentSpanId},
		{Name: ColTraceState, Data: traceState},
		{Name: ColSpanName, Data: spanName},
		{Name: ColSpanKind, Data: spanKind},
		{Name: ColServiceName, Data: serviceName},
		{Name: ColResourceAttributesBoolKeys, Data: resourceAttributesBoolKeys},
		{Name: ColResourceAttributesBoolValues, Data: resourceAttributesBoolValues},
		{Name: ColResourceAttributesDoubleKeys, Data: resourceAttributesDoubleKeys},
		{Name: ColResourceAttributesDoubleValues, Data: resourceAttributesDoubleValues},
		{Name: ColResourceAttributesIntKeys, Data: resourceAttributesIntKeys},
		{Name: ColResourceAttributesIntValues, Data: resourceAttributesIntValues},
		{Name: ColResourceAttributesStrKeys, Data: resourceAttributesStrKeys},
		{Name: ColResourceAttributesStrValues, Data: resourceAttributesStrValues},
		{Name: ColResourceAttributesEmptyKeys, Data: resourceAttributesEmptyKeys},
		{Name: ColResourceAttributesEmptyMapKeys, Data: resourceAttributesEmptyMapKeys},
		{Name: ColResourceAttributesEmptySlicesKeys, Data: resourceAttributesEmptySliceKeys},
		{Name: ColResourceAttributesEmptyBytesKeys, Data: resourceAttributesEmptyBytesKeys},
		{Name: ColScopeName, Data: scopeName},
		{Name: ColScopeVersion, Data: scopeVersion},
		{Name: ColScopeAttributesBoolKeys, Data: scopeAttributesBoolKeys},
		{Name: ColScopeAttributesBoolValues, Data: scopeAttributesBoolValues},
		{Name: ColScopeAttributesDoubleKeys, Data: scopeAttributesDoubleKeys},
		{Name: ColScopeAttributesDoubleValues, Data: scopeAttributesDoubleValues},
		{Name: ColScopeAttributesIntKeys, Data: scopeAttributesIntKeys},
		{Name: ColScopeAttributesIntValues, Data: scopeAttributesIntValues},
		{Name: ColScopeAttributesStrKeys, Data: scopeAttributesStrKeys},
		{Name: ColScopeAttributesStrValues, Data: scopeAttributesStrValues},
		{Name: ColScopeAttributesEmptyKeys, Data: scopeAttributesEmptyKeys},
		{Name: ColScopeAttributesEmptyMapKeys, Data: scopeAttributesEmptyMapKeys},
		{Name: ColScopeAttributesEmptySlicesKeys, Data: scopeAttributesEmptySliceKeys},
		{Name: ColScopeAttributesEmptyBytesKeys, Data: scopeAttributesEmptyBytesKeys},
		{Name: ColSpanAttributesBoolKeys, Data: spanAttributesBoolKeys},
		{Name: ColSpanAttributesBoolValues, Data: spanAttributesBoolValues},
		{Name: ColSpanAttributesDoubleKeys, Data: spanAttributesDoubleKeys},
		{Name: ColSpanAttributesDoubleValues, Data: spanAttributesDoubleValues},
		{Name: ColSpanAttributesIntKeys, Data: spanAttributesIntKeys},
		{Name: ColSpanAttributesIntValues, Data: spanAttributesIntValues},
		{Name: ColSpanAttributesStrKeys, Data: spanAttributesStrKeys},
		{Name: ColSpanAttributesStrValues, Data: spanAttributesStrValues},
		{Name: ColSpanAttributesEmptyKeys, Data: spanAttributesEmptyKeys},
		{Name: ColSpanAttributesEmptyMapKeys, Data: spanAttributesEmptyMapKeys},
		{Name: ColSpanAttributesEmptySlicesKeys, Data: spanAttributesEmptySliceKeys},
		{Name: ColSpanAttributesEmptyBytesKeys, Data: spanAttributesEmptyBytesKeys},
		{Name: ColDuration, Data: duration},
		{Name: ColStatusCode, Data: statusCode},
		{Name: ColStatusMessage, Data: statusMessage},
		{Name: ColEventsTimestamp, Data: eventsTimestamp},
		{Name: ColEventsName, Data: eventsName},
		{Name: ColEventsAttributesBoolKeys, Data: eventsAttributesBoolKeys},
		{Name: ColEventsAttributesBoolValues, Data: eventsAttributesBoolValues},
		{Name: ColEventsAttributesDoubleKeys, Data: eventsAttributesDoubleKeys},
		{Name: ColEventsAttributesDoubleValues, Data: eventsAttributesDoubleValues},
		{Name: ColEventsAttributesIntKeys, Data: eventsAttributesIntKeys},
		{Name: ColEventsAttributesIntValues, Data: eventsAttributesIntValues},
		{Name: ColEventsAttributesStrKeys, Data: eventsAttributesStrKeys},
		{Name: ColEventsAttributesStrValues, Data: eventsAttributesStrValues},
		{Name: ColEventsAttributesEmptyKeys, Data: eventsAttributesEmptyKeys},
		{Name: ColEventsAttributesEmptyMapKeys, Data: eventsAttributesEmptyMapKeys},
		{Name: ColEventsAttributesEmptySlicesKeys, Data: eventsAttributesEmptySlicesKeys},
		{Name: ColEventsAttributesEmptyBytesKeys, Data: eventsAttributesEmptyBytesKeys},
		{Name: ColLinksTraceId, Data: linksTraceId},
		{Name: ColLinksSpanId, Data: linksSpanId},
		{Name: ColLinksTraceState, Data: linksTraceState},
		{Name: ColLinksAttributesBoolKeys, Data: linksAttributesBoolKeys},
		{Name: ColLinksAttributesBoolValues, Data: linksAttributesBoolValues},
		{Name: ColLinksAttributesDoubleKeys, Data: linksAttributesDoubleKeys},
		{Name: ColLinksAttributesDoubleValues, Data: linksAttributesDoubleValues},
		{Name: ColLinksAttributesIntKeys, Data: linksAttributesIntKeys},
		{Name: ColLinksAttributesIntValues, Data: linksAttributesIntValues},
		{Name: ColLinksAttributesStrKeys, Data: linksAttributesStrKeys},
		{Name: ColLinksAttributesStrValues, Data: linksAttributesStrValues},
		{Name: ColLinksAttributesEmptyKeys, Data: linksAttributesEmptyKeys},
		{Name: ColLinksAttributesEmptyMapKeys, Data: linksAttributesEmptyMapKeys},
		{Name: ColLinksAttributesEmptySlicesKeys, Data: linksAttributesEmptySlicesKeys},
		{Name: ColLinksAttributesEmptyBytesKeys, Data: linksAttributesEmptyBytesKeys},
	}
}

// appendAttributes: Writes a complete pcommon.Map to the database. AttributesGroup and pcommon.Map have a one-to-one relationship.
func appendAttributes(attrType AttributeType, group AttributesGroup) {
	switch attrType {
	case AttributeTypeResource:
		resourceAttributesBoolKeys.Append(group.BoolKeys)
		resourceAttributesBoolValues.Append(group.BoolValues)
		resourceAttributesDoubleKeys.Append(group.DoubleKeys)
		resourceAttributesDoubleValues.Append(group.DoubleValues)
		resourceAttributesIntKeys.Append(group.IntKeys)
		resourceAttributesIntValues.Append(group.IntValues)
		resourceAttributesStrKeys.Append(group.StrKeys)
		resourceAttributesStrValues.Append(group.StrValues)
		resourceAttributesEmptyKeys.Append(group.EmptyKeys)
		resourceAttributesEmptyMapKeys.Append(group.EmptyMapKeys)
		resourceAttributesEmptySliceKeys.Append(group.EmptySliceKeys)
		resourceAttributesEmptyBytesKeys.Append(group.EmptyBytesKeys)
	case AttributeTypeScope:
		scopeAttributesBoolKeys.Append(group.BoolKeys)
		scopeAttributesBoolValues.Append(group.BoolValues)
		scopeAttributesDoubleKeys.Append(group.DoubleKeys)
		scopeAttributesDoubleValues.Append(group.DoubleValues)
		scopeAttributesIntKeys.Append(group.IntKeys)
		scopeAttributesIntValues.Append(group.IntValues)
		scopeAttributesStrKeys.Append(group.StrKeys)
		scopeAttributesStrValues.Append(group.StrValues)
		scopeAttributesEmptyKeys.Append(group.EmptyKeys)
		scopeAttributesEmptyMapKeys.Append(group.EmptyMapKeys)
		scopeAttributesEmptySliceKeys.Append(group.EmptySliceKeys)
		scopeAttributesEmptyBytesKeys.Append(group.EmptyBytesKeys)
	case AttributeTypeSpan:
		spanAttributesBoolKeys.Append(group.BoolKeys)
		spanAttributesBoolValues.Append(group.BoolValues)
		spanAttributesDoubleKeys.Append(group.DoubleKeys)
		spanAttributesDoubleValues.Append(group.DoubleValues)
		spanAttributesIntKeys.Append(group.IntKeys)
		spanAttributesIntValues.Append(group.IntValues)
		spanAttributesStrKeys.Append(group.StrKeys)
		spanAttributesStrValues.Append(group.StrValues)
		spanAttributesEmptyKeys.Append(group.EmptyKeys)
		spanAttributesEmptyMapKeys.Append(group.EmptyMapKeys)
		spanAttributesEmptySliceKeys.Append(group.EmptySliceKeys)
		spanAttributesEmptyBytesKeys.Append(group.EmptyBytesKeys)
	}
}

// appendNestedAttributes: Writes a complete set of pcommon.Map to the database. NestedAttributesGroup and pcommon.Map have a one-to-many relationship.
func appendNestedAttributes(attrType AttributeType, group NestedAttributesGroup) {
	switch attrType {
	case AttributeTypeEvent:
		eventsAttributesBoolKeys.Append(group.BoolKeys)
		eventsAttributesBoolValues.Append(group.BoolValues)
		eventsAttributesDoubleKeys.Append(group.DoubleKeys)
		eventsAttributesDoubleValues.Append(group.DoubleValues)
		eventsAttributesIntKeys.Append(group.IntKeys)
		eventsAttributesIntValues.Append(group.IntValues)
		eventsAttributesStrKeys.Append(group.StrKeys)
		eventsAttributesStrValues.Append(group.StrValues)
		eventsAttributesEmptyKeys.Append(group.EmptyKeys)
		eventsAttributesEmptyMapKeys.Append(group.EmptyMapKeys)
		eventsAttributesEmptySlicesKeys.Append(group.EmptySliceKeys)
		eventsAttributesEmptyBytesKeys.Append(group.EmptyBytesKeys)
	case AttributeTypeLink:
		linksAttributesBoolKeys.Append(group.BoolKeys)
		linksAttributesBoolValues.Append(group.BoolValues)
		linksAttributesDoubleKeys.Append(group.DoubleKeys)
		linksAttributesDoubleValues.Append(group.DoubleValues)
		linksAttributesIntKeys.Append(group.IntKeys)
		linksAttributesIntValues.Append(group.IntValues)
		linksAttributesStrKeys.Append(group.StrKeys)
		linksAttributesStrValues.Append(group.StrValues)
		linksAttributesEmptyKeys.Append(group.EmptyKeys)
		linksAttributesEmptyMapKeys.Append(group.EmptyMapKeys)
		linksAttributesEmptySlicesKeys.Append(group.EmptySliceKeys)
		linksAttributesEmptyBytesKeys.Append(group.EmptyBytesKeys)
	}
}

func getServiceName(resAttr pcommon.Map) string {
	var serviceName string
	if v, ok := resAttr.Get(conventions.AttributeServiceName); ok {
		serviceName = v.AsString()
	}
	return serviceName
}

func traceIDToHexOrEmptyString(id pcommon.TraceID) string {
	return hex.EncodeToString(id[:])
}

func spanIDToHexOrEmptyString(id pcommon.SpanID) string {
	return hex.EncodeToString(id[:])
}
