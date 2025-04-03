// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"github.com/ClickHouse/ch-go/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var (
	scopeAttributesBytesKey   = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesSliceKey   = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesMapKey     = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesBytesValue = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesSliceValue = new(proto.ColJSONStr).LowCardinality().Array()
	scopeAttributesMapValue   = new(proto.ColJSONStr).LowCardinality().Array()

	ColumnScopeAttributesBytesKey   = "scopeAttributesBytesKey"
	ColumnScopeAttributesSliceKey   = "scopeAttributesSliceKey"
	ColumnScopeAttributesMapKey     = "scopeAttributesMapKey"
	ColumnScopeAttributesBytesValue = "scopeAttributesBytesValue"
	ColumnScopeAttributesSliceValue = "scopeAttributesSliceValue"
	ColumnScopeAttributesMapValue   = "scopeAttributesMapValue"
)

var (
	resourceAttributesBytesKey   = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesSliceKey   = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesMapKey     = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesBytesValue = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesSliceValue = new(proto.ColJSONStr).LowCardinality().Array()
	resourceAttributesMapValue   = new(proto.ColJSONStr).LowCardinality().Array()

	ColumnResourceAttributesBytesKey   = "resourceAttributesBytesKey"
	ColumnResourceAttributesSliceKey   = "resourceAttributesSliceKey"
	ColumnResourceAttributesMapKey     = "resourceAttributesMapKey"
	ColumnResourceAttributesBytesValue = "resourceAttributesBytesValue"
	ColumnResourceAttributesSliceValue = "resourceAttributesSliceValue"
	ColumnResourceAttributesMapValue   = "resourceAttributesMapValue"
)

var (
	spanAttributesBytesKey   = new(proto.ColStr).LowCardinality().Array()
	spanAttributesSliceKey   = new(proto.ColStr).LowCardinality().Array()
	spanAttributesMapKey     = new(proto.ColStr).LowCardinality().Array()
	spanAttributesBytesValue = new(proto.ColStr).LowCardinality().Array()
	spanAttributesSliceValue = new(proto.ColJSONStr).LowCardinality().Array()
	spanAttributesMapValue   = new(proto.ColJSONStr).LowCardinality().Array()

	ColumnSpanAttributesBytesKey   = "spanAttributesBytesKey"
	ColumnSpanAttributesSliceKey   = "spanAttributesSliceKey"
	ColumnSpanAttributesMapKey     = "spanAttributesMapKey"
	ColumnSpanAttributesBytesValue = "spanAttributesBytesValue"
	ColumnSpanAttributesSliceValue = "spanAttributesSliceValue"
	ColumnSpanAttributesMapValue   = "spanAttributesMapValue"
)

var (
	eventsAttributesBytesKeys    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesSlicesKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesMapKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesBytesValue   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesSlicesValues = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())
	eventsAttributeMapValues     = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())

	ColumnEventsAttributesBytesKeys   = "eventsAttributesBytesKeys"
	ColumnEventsAttributesSliceKeys   = "eventsAttributesSliceKeys"
	ColumnEventsAttributesMapKeys     = "eventsAttributesMapKeys"
	ColumnEventsAttributesBytesValues = "eventsAttributesBytesValues"
	ColumnEventsAttributesSliceValues = "eventsAttributesSliceValues"
	ColumnEventsAttributesMapValues   = "eventsAttributesMapValues"
)

var (
	linksAttributesBytesKeys    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesSlicesKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesMapKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesBytesValue   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesSlicesValues = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())
	linksAttributeMapValues     = proto.NewArray(new(proto.ColJSONStr).LowCardinality().Array())

	ColumnLinksAttributesBytesKeys   = "linksAttributesBytesKeys"
	ColumnLinksAttributesSliceKeys   = "linksAttributesSliceKeys"
	ColumnLinksAttributesMapKeys     = "linksAttributesMapKeys"
	ColumnLinksAttributesBytesValues = "linksAttributesBytesValues"
	ColumnLinksAttributesSliceValues = "linksAttributesSliceValues"
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
		{Name: ColumnResourceAttributesSliceKey, Data: resourceAttributesSliceKey},
		{Name: ColumnResourceAttributesMapKey, Data: resourceAttributesMapKey},
		{Name: ColumnResourceAttributesBytesValue, Data: resourceAttributesBytesValue},
		{Name: ColumnResourceAttributesSliceValue, Data: resourceAttributesSliceValue},
		{Name: ColumnResourceAttributesMapValue, Data: resourceAttributesMapValue},

		{Name: ColumnScopeAttributesBytesKey, Data: scopeAttributesBytesKey},
		{Name: ColumnScopeAttributesSliceKey, Data: scopeAttributesSliceKey},
		{Name: ColumnScopeAttributesMapKey, Data: scopeAttributesMapKey},
		{Name: ColumnScopeAttributesBytesValue, Data: scopeAttributesBytesValue},
		{Name: ColumnScopeAttributesSliceValue, Data: scopeAttributesSliceValue},
		{Name: ColumnScopeAttributesMapValue, Data: scopeAttributesMapValue},

		{Name: ColumnSpanAttributesBytesKey, Data: spanAttributesBytesKey},
		{Name: ColumnSpanAttributesSliceKey, Data: spanAttributesSliceKey},
		{Name: ColumnSpanAttributesMapKey, Data: spanAttributesMapKey},
		{Name: ColumnSpanAttributesBytesValue, Data: spanAttributesBytesValue},
		{Name: ColumnSpanAttributesSliceValue, Data: spanAttributesSliceValue},
		{Name: ColumnSpanAttributesMapValue, Data: spanAttributesMapValue},

		{Name: ColumnEventsAttributesBytesKeys, Data: eventsAttributesBytesKeys},
		{Name: ColumnEventsAttributesSliceKeys, Data: eventsAttributesSlicesKeys},
		{Name: ColumnEventsAttributesMapKeys, Data: eventsAttributesMapKeys},
		{Name: ColumnEventsAttributesBytesValues, Data: eventsAttributesBytesValue},
		{Name: ColumnEventsAttributesSliceValues, Data: eventsAttributesSlicesValues},
		{Name: ColumnEventsAttributesMapValues, Data: eventsAttributeMapValues},

		{Name: ColumnLinksAttributesBytesKeys, Data: linksAttributesBytesKeys},
		{Name: ColumnLinksAttributesSliceKeys, Data: linksAttributesSlicesKeys},
		{Name: ColumnLinksAttributesMapKeys, Data: linksAttributesMapKeys},
		{Name: ColumnLinksAttributesBytesValues, Data: linksAttributesBytesValue},
		{Name: ColumnLinksAttributesSliceValues, Data: linksAttributesSlicesValues},
		{Name: ColumnLinksAttributesMapValues, Data: linksAttributeMapValues},
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
