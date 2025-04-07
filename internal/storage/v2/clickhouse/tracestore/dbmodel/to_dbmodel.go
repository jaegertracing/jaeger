// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"github.com/ClickHouse/ch-go/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// NestedAttributesGroup There is a one-to-many relationship between a NestedAttributesGroup and a pcommon.Map.
// In the official ClickHouse implementation, ptrace.SpanEventSlice and ptrace.SpanLinkSlice are stored in a Nested format in the database.
// Since all arrays in Nested need to have the same length, AttributesGroup cannot be used directly.
type NestedAttributesGroup struct {
	AttributesGroups []AttributesGroup
}

// AttributesGroup There is a one-to-one relationship between an AttributesGroup and a pcommon.Map.
// In the official ClickHouse implementation, pcommon.Map is stored as a string Map in the database for both key and value,
// which causes the loss of data types. See:
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/4930224211ea876c71367cf04038e23359c0338a/exporter/clickhouseexporter/exporter_traces.go#L162
// Therefore, splitting the key and value of pcommon.Map into arrays for storage ensures that 1. data types are not lost
// and 2. it can be more conveniently used as query parameters.
type AttributesGroup struct {
	BytesKeys   []string
	BytesValues []string
	MapKeys     []string
	MapValues   []string
	SliceKeys   []string
	SliceValues []string
}

// AttributesToGroup categorizes and aggregates Attributes based on the data type of their values, and writes them in batches.
func AttributesToGroup(attributes pcommon.Map) AttributesGroup {
	attributesMap := AttributesToMap(attributes)
	var group AttributesGroup
	for valueType := range attributesMap {
		kvPairs := attributesMap[valueType]
		switch valueType {
		case ValueTypeSlice:
			for k, v := range kvPairs {
				group.SliceKeys = append(group.SliceKeys, k)
				group.SliceValues = append(group.SliceValues, v)
			}
		case ValueTypeMap:
			for k, v := range kvPairs {
				group.MapKeys = append(group.MapKeys, k)
				group.MapValues = append(group.MapValues, v)
			}
		case ValueTypeBytes:
			for k, v := range kvPairs {
				group.BytesKeys = append(group.BytesKeys, k)
				group.BytesValues = append(group.BytesValues, v)
			}
		default:
		}
	}

	return group
}

// AttributesToMap Groups a pcommon.Map by data type and splits the key-value pairs into arrays for storage.
// The values in the key-value pairs of a pcommon.Map instance may not all be of the same data type.
// For example, a pcommon.Map can contain key-value pairs such as:
// string-string, string-bool, string-int64, string-float64. Clearly, the key-value pairs need to be classified based on the data type.
func AttributesToMap(attrs pcommon.Map) map[pcommon.ValueType]map[string]string {
	result := make(map[pcommon.ValueType]map[string]string)
	for _, valueType := range []pcommon.ValueType{
		ValueTypeBool, ValueTypeDouble, ValueTypeInt, ValueTypeStr,
		ValueTypeMap, ValueTypeSlice, ValueTypeBytes,
	} {
		result[valueType] = make(map[string]string)
	}
	// Fill according to the data type of the value
	attrs.Range(func(k string, v pcommon.Value) bool {
		typ := v.Type()
		// For basic data types (such as bool, uint64, and float64), type safety is guaranteed when converting back from a string.
		// For non-basic types (such as Map, Slice, and []byte), they are serialized and stored as JSON-formatted strings,
		// ensuring that the original type is preserved when reading
		result[typ][k] = v.AsString()
		return true
	})
	return result
}

var (
	scopeAttributesBytesKey   = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesBytesValue = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesSliceKey   = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesSliceValue = new(proto.ColJSONStr).LowCardinality().Array()
	scopeAttributesMapKey     = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesMapValue   = new(proto.ColJSONStr).LowCardinality().Array()

	ColumnScopeAttributesBytesKey   = "scopeAttributesBytesKey"
	ColumnScopeAttributesBytesValue = "scopeAttributesBytesValue"
	ColumnScopeAttributesSliceKey   = "scopeAttributesSliceKey"
	ColumnScopeAttributesSliceValue = "scopeAttributesSliceValue"
	ColumnScopeAttributesMapKey     = "scopeAttributesMapKey"
	ColumnScopeAttributesMapValue   = "scopeAttributesMapValue"
)

var (
	resourceAttributesBytesKey   = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesBytesValue = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesSliceKey   = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesSliceValue = new(proto.ColJSONStr).LowCardinality().Array()
	resourceAttributesMapKey     = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesMapValue   = new(proto.ColJSONStr).LowCardinality().Array()

	ColumnResourceAttributesBytesKey   = "resourceAttributesBytesKey"
	ColumnResourceAttributesBytesValue = "resourceAttributesBytesValue"
	ColumnResourceAttributesSliceKey   = "resourceAttributesSliceKey"
	ColumnResourceAttributesSliceValue = "resourceAttributesSliceValue"
	ColumnResourceAttributesMapKey     = "resourceAttributesMapKey"
	ColumnResourceAttributesMapValue   = "resourceAttributesMapValue"
)

var (
	spanAttributesBytesKey   = new(proto.ColStr).LowCardinality().Array()
	spanAttributesBytesValue = new(proto.ColStr).LowCardinality().Array()
	spanAttributesSliceKey   = new(proto.ColStr).LowCardinality().Array()
	spanAttributesSliceValue = new(proto.ColJSONStr).LowCardinality().Array()
	spanAttributesMapKey     = new(proto.ColStr).LowCardinality().Array()
	spanAttributesMapValue   = new(proto.ColJSONStr).LowCardinality().Array()

	ColumnSpanAttributesBytesKey   = "spanAttributesBytesKey"
	ColumnSpanAttributesBytesValue = "spanAttributesBytesValue"
	ColumnSpanAttributesSliceKey   = "spanAttributesSliceKey"
	ColumnSpanAttributesSliceValue = "spanAttributesSliceValue"
	ColumnSpanAttributesMapKey     = "spanAttributesMapKey"
	ColumnSpanAttributesMapValue   = "spanAttributesMapValue"
)

var (
	eventsAttributesBytesKeys    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesBytesValue   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesSlicesKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesSlicesValues = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())
	eventsAttributesMapKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributeMapValues     = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())

	ColumnEventsAttributesBytesKeys   = "eventsAttributesBytesKeys"
	ColumnEventsAttributesBytesValues = "eventsAttributesBytesValues"
	ColumnEventsAttributesSliceKeys   = "eventsAttributesSliceKeys"
	ColumnEventsAttributesSliceValues = "eventsAttributesSliceValues"
	ColumnEventsAttributesMapKeys     = "eventsAttributesMapKeys"
	ColumnEventsAttributesMapValues   = "eventsAttributesMapValues"
)

var (
	linksAttributesBytesKeys    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesBytesValue   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesSlicesKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesSlicesValues = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())
	linksAttributesMapKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributeMapValues     = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())

	ColumnLinksAttributesBytesKeys   = "linksAttributesBytesKeys"
	ColumnLinksAttributesBytesValues = "linksAttributesBytesValues"
	ColumnLinksAttributesSliceKeys   = "linksAttributesSliceKeys"
	ColumnLinksAttributesSliceValues = "linksAttributesSliceValues"
	ColumnLinksAttributesMapKeys     = "linksAttributesMapKeys"
	ColumnLinksAttributesMapValues   = "linksAttributesMapValues"
)

type AttributeType int32

const (
	AttributeTypeResource AttributeType = iota
	AttributeTypeScope
	AttributeTypeSpan
	AttributeTypeEvent
	AttributeTypeLink
)

// FromPtrace converts the OpenTelemetry Traces model into a ClickHouse-compatible format for batch insertion.
// It maps the trace attributes, spans, links and events from the OTel model to the appropriate ClickHouse column types
func FromPtrace(td ptrace.Traces) proto.Input {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		resourceSpans := td.ResourceSpans().At(i)
		resourceGroup := AttributesToGroup(resourceSpans.Resource().Attributes())

		for j := 0; j < resourceSpans.ScopeSpans().Len(); j++ {
			scope := resourceSpans.ScopeSpans().At(j).Scope()
			scopeGroup := AttributesToGroup(scope.Attributes())

			spans := resourceSpans.ScopeSpans().At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				spanGroup := AttributesToGroup(span.Attributes())
				AppendGroup(AttributeTypeResource, resourceGroup)
				AppendGroup(AttributeTypeScope, scopeGroup)
				AppendGroup(AttributeTypeSpan, spanGroup)

				var eventNestedGroup NestedAttributesGroup
				for l := 0; l < span.Events().Len(); l++ {
					event := span.Events().At(l)
					eventGroup := AttributesToGroup(event.Attributes())
					eventNestedGroup.AttributesGroups = append(eventNestedGroup.AttributesGroups, eventGroup)
				}
				AppendGroups(AttributeTypeEvent, eventNestedGroup)

				var linkNestedGroup NestedAttributesGroup
				for l := 0; l < span.Links().Len(); l++ {
					link := span.Links().At(l)
					linkGroup := AttributesToGroup(link.Attributes())
					linkNestedGroup.AttributesGroups = append(linkNestedGroup.AttributesGroups, linkGroup)
				}
				AppendGroups(AttributeTypeLink, linkNestedGroup)
			}
		}
	}

	return proto.Input{
		{Name: ColumnResourceAttributesBytesKey, Data: resourceAttributesBytesKey},
		{Name: ColumnResourceAttributesBytesValue, Data: resourceAttributesBytesValue},
		{Name: ColumnResourceAttributesMapKey, Data: resourceAttributesMapKey},
		{Name: ColumnResourceAttributesMapValue, Data: resourceAttributesMapValue},
		{Name: ColumnResourceAttributesSliceKey, Data: resourceAttributesSliceKey},
		{Name: ColumnResourceAttributesSliceValue, Data: resourceAttributesSliceValue},

		{Name: ColumnScopeAttributesBytesKey, Data: scopeAttributesBytesKey},
		{Name: ColumnScopeAttributesBytesValue, Data: scopeAttributesBytesValue},
		{Name: ColumnScopeAttributesMapKey, Data: scopeAttributesMapKey},
		{Name: ColumnScopeAttributesMapValue, Data: scopeAttributesMapValue},
		{Name: ColumnScopeAttributesSliceKey, Data: scopeAttributesSliceKey},
		{Name: ColumnScopeAttributesSliceValue, Data: scopeAttributesSliceValue},

		{Name: ColumnSpanAttributesBytesKey, Data: spanAttributesBytesKey},
		{Name: ColumnSpanAttributesBytesValue, Data: spanAttributesBytesValue},
		{Name: ColumnSpanAttributesMapKey, Data: spanAttributesMapKey},
		{Name: ColumnSpanAttributesMapValue, Data: spanAttributesMapValue},
		{Name: ColumnSpanAttributesSliceKey, Data: spanAttributesSliceKey},
		{Name: ColumnSpanAttributesSliceValue, Data: spanAttributesSliceValue},

		{Name: ColumnEventsAttributesBytesKeys, Data: eventsAttributesBytesKeys},
		{Name: ColumnEventsAttributesBytesValues, Data: eventsAttributesBytesValue},
		{Name: ColumnEventsAttributesMapKeys, Data: eventsAttributesMapKeys},
		{Name: ColumnEventsAttributesMapValues, Data: eventsAttributeMapValues},
		{Name: ColumnEventsAttributesSliceKeys, Data: eventsAttributesSlicesKeys},
		{Name: ColumnEventsAttributesSliceValues, Data: eventsAttributesSlicesValues},

		{Name: ColumnLinksAttributesBytesKeys, Data: linksAttributesBytesKeys},
		{Name: ColumnLinksAttributesBytesValues, Data: linksAttributesBytesValue},
		{Name: ColumnLinksAttributesMapKeys, Data: linksAttributesMapKeys},
		{Name: ColumnLinksAttributesMapValues, Data: linksAttributeMapValues},
		{Name: ColumnLinksAttributesSliceKeys, Data: linksAttributesSlicesKeys},
		{Name: ColumnLinksAttributesSliceValues, Data: linksAttributesSlicesValues},
	}
}

const (
	ValueTypeBool   = pcommon.ValueTypeBool
	ValueTypeDouble = pcommon.ValueTypeDouble
	ValueTypeInt    = pcommon.ValueTypeInt
	ValueTypeStr    = pcommon.ValueTypeStr
	ValueTypeMap    = pcommon.ValueTypeMap
	ValueTypeSlice  = pcommon.ValueTypeSlice
	ValueTypeBytes  = pcommon.ValueTypeBytes
)

// AppendGroups Writes a complete set of pcommon.Map to the database. NestedAttributesGroup and pcommon.Map have a one-to-many relationship.
func AppendGroups(attributeType AttributeType, nestedGroup NestedAttributesGroup) {
	var byteKeys [][]string
	var bytesValues [][]string
	var mapKeys [][]string
	var mapValues [][]string
	var sliceKeys [][]string
	var sliceValues [][]string
	for _, group := range nestedGroup.AttributesGroups {
		byteKeys = append(byteKeys, group.BytesKeys)
		bytesValues = append(bytesValues, group.BytesValues)
		sliceKeys = append(sliceKeys, group.SliceKeys)
		sliceValues = append(sliceValues, group.SliceValues)
		mapKeys = append(mapKeys, group.MapKeys)
		mapValues = append(mapValues, group.MapValues)
	}
	switch attributeType {
	case AttributeTypeEvent:
		eventsAttributesBytesKeys.Append(byteKeys)
		eventsAttributesSlicesKeys.Append(sliceKeys)
		eventsAttributesMapKeys.Append(mapKeys)
		eventsAttributesBytesValue.Append(bytesValues)
		eventsAttributesSlicesValues.Append(sliceValues)
		eventsAttributeMapValues.Append(mapValues)
	case AttributeTypeLink:
		linksAttributesBytesKeys.Append(byteKeys)
		linksAttributesSlicesKeys.Append(sliceKeys)
		linksAttributesMapKeys.Append(mapKeys)
		linksAttributesBytesValue.Append(bytesValues)
		linksAttributesSlicesValues.Append(sliceValues)
		linksAttributeMapValues.Append(mapValues)
	}
}

// AppendGroup Writes a complete pcommon.Map to the database. AttributesGroup and pcommon.Map have a one-to-one relationship.
func AppendGroup(attributeType AttributeType, group AttributesGroup) {
	switch attributeType {
	case AttributeTypeResource:
		resourceAttributesBytesKey.Append(group.BytesKeys)
		resourceAttributesSliceKey.Append(group.SliceKeys)
		resourceAttributesMapKey.Append(group.MapKeys)
		resourceAttributesBytesValue.Append(group.BytesValues)
		resourceAttributesSliceValue.Append(group.SliceValues)
		resourceAttributesMapValue.Append(group.MapValues)
	case AttributeTypeScope:
		scopeAttributesBytesKey.Append(group.BytesKeys)
		scopeAttributesSliceKey.Append(group.SliceKeys)
		scopeAttributesMapKey.Append(group.MapKeys)
		scopeAttributesBytesValue.Append(group.BytesValues)
		scopeAttributesSliceValue.Append(group.SliceValues)
		scopeAttributesMapValue.Append(group.MapValues)
	case AttributeTypeSpan:
		spanAttributesBytesKey.Append(group.BytesKeys)
		spanAttributesSliceKey.Append(group.SliceKeys)
		spanAttributesMapKey.Append(group.MapKeys)
		spanAttributesBytesValue.Append(group.BytesValues)
		spanAttributesSliceValue.Append(group.SliceValues)
		spanAttributesMapValue.Append(group.MapValues)
	}
}
