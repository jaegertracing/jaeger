// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"

	"github.com/ClickHouse/ch-go/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// NestedAttributesGroup There is a one-to-many relationship between a NestedAttributesGroup and a pcommon.Map.
// In the OTel Collector Contrib implementation, ptrace.SpanEventSlice and ptrace.SpanLinkSlice are stored in a Nested format in the database.
// Since all arrays in Nested need to have the same length, AttributesGroup cannot be used directly.
type NestedAttributesGroup struct {
	AttributesGroups []AttributesGroup
}

// AttributesGroup There is a one-to-one relationship between an AttributesGroup and a pcommon.Map.
// In the OTel Collector Contrib implementation, pcommon.Map is stored as a string Map in the database for both key and value,
// which causes the loss of data types. See:
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/4930224211ea876c71367cf04038e23359c0338a/exporter/clickhouseexporter/exporter_traces.go#L162
// Therefore, splitting the key and value of pcommon.Map into arrays for storage ensures that 1. data types are not lost
// and 2. it can be more conveniently used as query parameters.
type AttributesGroup struct {
	BoolKeys     []string
	BoolValues   []bool
	DoubleKeys   []string
	DoubleValues []float64
	IntKeys      []string
	IntValues    []int64
	StrKeys      []string
	StrValues    []string
	BytesKeys    []string
	BytesValues  []string
}

// AttributesToGroup Categorizes and aggregates Attributes based on the data type of their values, and writes them in batches.
func AttributesToGroup(attributes pcommon.Map) AttributesGroup {
	attributesMap := AttributesToMap(attributes)
	var group AttributesGroup
	for valueType := range attributesMap {
		kvPairs := attributesMap[valueType]
		switch valueType {
		case ValueTypeBool:
			for k, v := range kvPairs {
				group.BoolKeys = append(group.BoolKeys, k)
				group.BoolValues = append(group.BoolValues, v.Bool())
			}
		case ValueTypeDouble:
			for k, v := range kvPairs {
				group.DoubleKeys = append(group.DoubleKeys, k)
				group.DoubleValues = append(group.DoubleValues, v.Double())
			}
		case ValueTypeInt:
			for k, v := range kvPairs {
				group.IntKeys = append(group.IntKeys, k)
				group.IntValues = append(group.IntValues, v.Int())
			}
		case ValueTypeStr:
			for k, v := range kvPairs {
				group.StrKeys = append(group.StrKeys, k)
				group.StrValues = append(group.StrValues, v.Str())
			}
		case ValueTypeBytes:
			for k, v := range kvPairs {
				group.BytesKeys = append(group.BytesKeys, k)
				byteStr := base64.StdEncoding.EncodeToString(v.Bytes().AsRaw())
				group.BytesValues = append(group.BytesValues, byteStr)
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
func AttributesToMap(attrs pcommon.Map) map[pcommon.ValueType]map[string]pcommon.Value {
	result := make(map[pcommon.ValueType]map[string]pcommon.Value)
	for _, valueType := range []pcommon.ValueType{
		ValueTypeBool, ValueTypeDouble, ValueTypeInt, ValueTypeStr, ValueTypeBytes,
	} {
		result[valueType] = make(map[string]pcommon.Value)
	}
	// Fill according to the data type of the value
	for k, v := range attrs.All() {
		typ := v.Type()
		// For basic data types (such as bool, uint64, and float64) we can make sure type safe.
		// TODO: For non-basic types (such as Map, Slice), they should be serialized and stored as OTLP/JSON strings
		result[typ][k] = v
	}
	return result
}

var (
	resourceAttributesBoolKey     = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesBoolValue   = new(proto.ColBool).Array()
	resourceAttributesDoubleKey   = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesDoubleValue = new(proto.ColFloat64).Array()
	resourceAttributesIntKey      = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesIntValue    = new(proto.ColInt64).Array()
	resourceAttributesStrKey      = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesStrValue    = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesBytesKey    = new(proto.ColStr).LowCardinality().Array()
	resourceAttributesBytesValue  = new(proto.ColStr).LowCardinality().Array()

	ColumnResourceAttributesBoolKey     = "resourceAttributesBoolKey"
	ColumnResourceAttributesBoolValue   = "resourceAttributesBoolValue"
	ColumnResourceAttributesDoubleKey   = "resourceAttributesDoubleKey"
	ColumnResourceAttributesDoubleValue = "resourceAttributesDoubleValue"
	ColumnResourceAttributesIntKey      = "resourceAttributesIntKey"
	ColumnResourceAttributesIntValue    = "resourceAttributesIntValue"
	ColumnResourceAttributesStrKey      = "resourceAttributesStrKey"
	ColumnResourceAttributesStrValue    = "resourceAttributesStrValue"
	ColumnResourceAttributesBytesKey    = "resourceAttributesBytesKey"
	ColumnResourceAttributesBytesValue  = "resourceAttributesBytesValue"
)

var (
	scopeAttributesBoolKey     = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesBoolValue   = new(proto.ColBool).Array()
	scopeAttributesDoubleKey   = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesDoubleValue = new(proto.ColFloat64).Array()
	scopeAttributesIntKey      = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesIntValue    = new(proto.ColInt64).Array()
	scopeAttributesStrKey      = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesStrValue    = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesBytesKey    = new(proto.ColStr).LowCardinality().Array()
	scopeAttributesBytesValue  = new(proto.ColStr).LowCardinality().Array()

	ColumnScopeAttributesBoolKey     = "scopeAttributesBoolKey"
	ColumnScopeAttributesBoolValue   = "scopeAttributesBoolValue"
	ColumnScopeAttributesDoubleKey   = "scopeAttributesDoubleKey"
	ColumnScopeAttributesDoubleValue = "scopeAttributesDoubleValue"
	ColumnScopeAttributesIntKey      = "scopeAttributesIntKey"
	ColumnScopeAttributesIntValue    = "scopeAttributesIntValue"
	ColumnScopeAttributesStrKey      = "scopeAttributesStrKey"
	ColumnScopeAttributesStrValue    = "scopeAttributesStrValue"
	ColumnScopeAttributesBytesKey    = "scopeAttributesBytesKey"
	ColumnScopeAttributesBytesValue  = "scopeAttributesBytesValue"
)

var (
	spanAttributesBoolKey     = new(proto.ColStr).LowCardinality().Array()
	spanAttributesBoolValue   = new(proto.ColBool).Array()
	spanAttributesDoubleKey   = new(proto.ColStr).LowCardinality().Array()
	spanAttributesDoubleValue = new(proto.ColFloat64).Array()
	spanAttributesIntKey      = new(proto.ColStr).LowCardinality().Array()
	spanAttributesIntValue    = new(proto.ColInt64).Array()
	spanAttributesStrKey      = new(proto.ColStr).LowCardinality().Array()
	spanAttributesStrValue    = new(proto.ColStr).LowCardinality().Array()
	spanAttributesBytesKey    = new(proto.ColStr).LowCardinality().Array()
	spanAttributesBytesValue  = new(proto.ColStr).LowCardinality().Array()

	ColumnSpanAttributesBoolKey     = "spanAttributesBoolKey"
	ColumnSpanAttributesBoolValue   = "spanAttributesBoolValue"
	ColumnSpanAttributesDoubleKey   = "spanAttributesDoubleKey"
	ColumnSpanAttributesDoubleValue = "spanAttributesDoubleValue"
	ColumnSpanAttributesIntKey      = "spanAttributesIntKey"
	ColumnSpanAttributesIntValue    = "spanAttributesIntValue"
	ColumnSpanAttributesStrKey      = "spanAttributesStrKey"
	ColumnSpanAttributesStrValue    = "spanAttributesStrValue"
	ColumnSpanAttributesBytesKey    = "spanAttributesBytesKey"
	ColumnSpanAttributesBytesValue  = "spanAttributesBytesValue"
)

var (
	eventsAttributesBoolKeys     = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesBoolValues   = proto.NewArray(new(proto.ColBool).Array())
	eventsAttributesDoubleKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesDoubleValues = proto.NewArray(new(proto.ColFloat64).Array())
	eventsAttributesIntKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesIntValues    = proto.NewArray(new(proto.ColInt64).Array())
	eventsAttributesStrKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesStrValues    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesBytesKeys    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	eventsAttributesBytesValue   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())

	ColumnEventsAttributesBoolKeys     = "eventsAttributesBoolKeys"
	ColumnEventsAttributesBoolValues   = "eventsAttributesBoolValues"
	ColumnEventsAttributesDoubleKeys   = "eventsAttributesDoubleKeys"
	ColumnEventsAttributesDoubleValues = "eventsAttributesDoubleValues"
	ColumnEventsAttributesIntKeys      = "eventsAttributesIntKeys"
	ColumnEventsAttributesIntValues    = "eventsAttributesIntValues"
	ColumnEventsAttributesStrKeys      = "eventsAttributesStrKeys"
	ColumnEventsAttributesStrValues    = "eventsAttributesStrValues"
	ColumnEventsAttributesBytesKeys    = "eventsAttributesBytesKeys"
	ColumnEventsAttributesBytesValues  = "eventsAttributesBytesValues"
)

var (
	linksAttributesBoolKeys     = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesBoolValues   = proto.NewArray(new(proto.ColBool).Array())
	linksAttributesDoubleKeys   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesDoubleValues = proto.NewArray(new(proto.ColFloat64).Array())
	linksAttributesIntKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesIntValues    = proto.NewArray(new(proto.ColInt64).Array())
	linksAttributesStrKeys      = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesStrValues    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesBytesKeys    = proto.NewArray(new(proto.ColStr).LowCardinality().Array())
	linksAttributesBytesValue   = proto.NewArray(new(proto.ColStr).LowCardinality().Array())

	ColumnLinksAttributesBoolKeys     = "linksAttributesBoolKeys"
	ColumnLinksAttributesBoolValues   = "linksAttributesBoolValues"
	ColumnLinksAttributesDoubleKeys   = "linksAttributesDoubleKeys"
	ColumnLinksAttributesDoubleValues = "linksAttributesDoubleValues"
	ColumnLinksAttributesIntKeys      = "linksAttributesIntKeys"
	ColumnLinksAttributesIntValues    = "linksAttributesIntValues"
	ColumnLinksAttributesStrKeys      = "linksAttributesStrKeys"
	ColumnLinksAttributesStrValues    = "linksAttributesStrValues"
	ColumnLinksAttributesBytesKeys    = "linksAttributesBytesKeys"
	ColumnLinksAttributesBytesValues  = "linksAttributesBytesValues"
)

type AttributeType int32

const (
	AttributeTypeResource AttributeType = iota
	AttributeTypeScope
	AttributeTypeSpan
	AttributeTypeEvent
	AttributeTypeLink
)

// FromPtrace Converts the OTel pipeline Traces into a ClickHouse-compatible format for batch insertion.
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
		// Resource
		{Name: ColumnResourceAttributesBoolKey, Data: resourceAttributesBoolKey},
		{Name: ColumnResourceAttributesBoolValue, Data: resourceAttributesBoolValue},
		{Name: ColumnResourceAttributesDoubleKey, Data: resourceAttributesDoubleKey},
		{Name: ColumnResourceAttributesDoubleValue, Data: resourceAttributesDoubleValue},
		{Name: ColumnResourceAttributesIntKey, Data: resourceAttributesIntKey},
		{Name: ColumnResourceAttributesIntValue, Data: resourceAttributesIntValue},
		{Name: ColumnResourceAttributesStrKey, Data: resourceAttributesStrKey},
		{Name: ColumnResourceAttributesStrValue, Data: resourceAttributesStrValue},
		{Name: ColumnResourceAttributesBytesKey, Data: resourceAttributesBytesKey},
		{Name: ColumnResourceAttributesBytesValue, Data: resourceAttributesBytesValue},
		// Scope
		{Name: ColumnScopeAttributesBoolKey, Data: scopeAttributesBoolKey},
		{Name: ColumnScopeAttributesBoolValue, Data: scopeAttributesBoolValue},
		{Name: ColumnScopeAttributesDoubleKey, Data: scopeAttributesDoubleKey},
		{Name: ColumnScopeAttributesDoubleValue, Data: scopeAttributesDoubleValue},
		{Name: ColumnScopeAttributesIntKey, Data: scopeAttributesIntKey},
		{Name: ColumnScopeAttributesIntValue, Data: scopeAttributesIntValue},
		{Name: ColumnScopeAttributesStrKey, Data: scopeAttributesStrKey},
		{Name: ColumnScopeAttributesStrValue, Data: scopeAttributesStrValue},
		{Name: ColumnScopeAttributesBytesKey, Data: scopeAttributesBytesKey},
		{Name: ColumnScopeAttributesBytesValue, Data: scopeAttributesBytesValue},
		// Span
		{Name: ColumnSpanAttributesBoolKey, Data: spanAttributesBoolKey},
		{Name: ColumnSpanAttributesBoolValue, Data: spanAttributesBoolValue},
		{Name: ColumnSpanAttributesDoubleKey, Data: spanAttributesDoubleKey},
		{Name: ColumnSpanAttributesDoubleValue, Data: spanAttributesDoubleValue},
		{Name: ColumnSpanAttributesIntKey, Data: spanAttributesIntKey},
		{Name: ColumnSpanAttributesIntValue, Data: spanAttributesIntValue},
		{Name: ColumnSpanAttributesStrKey, Data: spanAttributesStrKey},
		{Name: ColumnSpanAttributesStrValue, Data: spanAttributesStrValue},
		{Name: ColumnSpanAttributesBytesKey, Data: spanAttributesBytesKey},
		{Name: ColumnSpanAttributesBytesValue, Data: spanAttributesBytesValue},
		// Events
		{Name: ColumnEventsAttributesBoolKeys, Data: eventsAttributesBoolKeys},
		{Name: ColumnEventsAttributesBoolValues, Data: eventsAttributesBoolValues},
		{Name: ColumnEventsAttributesDoubleKeys, Data: eventsAttributesDoubleKeys},
		{Name: ColumnEventsAttributesDoubleValues, Data: eventsAttributesDoubleValues},
		{Name: ColumnEventsAttributesIntKeys, Data: eventsAttributesIntKeys},
		{Name: ColumnEventsAttributesIntValues, Data: eventsAttributesIntValues},
		{Name: ColumnEventsAttributesStrKeys, Data: eventsAttributesStrKeys},
		{Name: ColumnEventsAttributesStrValues, Data: eventsAttributesStrValues},
		{Name: ColumnEventsAttributesBytesKeys, Data: eventsAttributesBytesKeys},
		{Name: ColumnEventsAttributesBytesValues, Data: eventsAttributesBytesValue},
		// Links
		{Name: ColumnLinksAttributesBoolKeys, Data: linksAttributesBoolKeys},
		{Name: ColumnLinksAttributesBoolValues, Data: linksAttributesBoolValues},
		{Name: ColumnLinksAttributesDoubleKeys, Data: linksAttributesDoubleKeys},
		{Name: ColumnLinksAttributesDoubleValues, Data: linksAttributesDoubleValues},
		{Name: ColumnLinksAttributesIntKeys, Data: linksAttributesIntKeys},
		{Name: ColumnLinksAttributesIntValues, Data: linksAttributesIntValues},
		{Name: ColumnLinksAttributesStrKeys, Data: linksAttributesStrKeys},
		{Name: ColumnLinksAttributesStrValues, Data: linksAttributesStrValues},
		{Name: ColumnLinksAttributesBytesKeys, Data: linksAttributesBytesKeys},
		{Name: ColumnLinksAttributesBytesValues, Data: linksAttributesBytesValue},
	}
}

const (
	ValueTypeBool   = pcommon.ValueTypeBool
	ValueTypeDouble = pcommon.ValueTypeDouble
	ValueTypeInt    = pcommon.ValueTypeInt
	ValueTypeStr    = pcommon.ValueTypeStr
	ValueTypeBytes  = pcommon.ValueTypeBytes
)

// AppendGroups Writes a complete set of pcommon.Map to the database. NestedAttributesGroup and pcommon.Map have a one-to-many relationship.
func AppendGroups(attributeType AttributeType, nestedGroup NestedAttributesGroup) {
	var boolKeys [][]string
	var boolValues [][]bool
	var doubleKeys [][]string
	var doubleValues [][]float64
	var intKeys [][]string
	var intValues [][]int64
	var strKeys [][]string
	var strValues [][]string
	var byteKeys [][]string
	var bytesValues [][]string
	for _, group := range nestedGroup.AttributesGroups {
		boolKeys = append(boolKeys, group.BoolKeys)
		boolValues = append(boolValues, group.BoolValues)
		doubleKeys = append(doubleKeys, group.DoubleKeys)
		doubleValues = append(doubleValues, group.DoubleValues)
		intKeys = append(intKeys, group.IntKeys)
		intValues = append(intValues, group.IntValues)
		strKeys = append(strKeys, group.StrKeys)
		strValues = append(strValues, group.StrValues)
		byteKeys = append(byteKeys, group.BytesKeys)
		bytesValues = append(bytesValues, group.BytesValues)
	}
	switch attributeType {
	case AttributeTypeEvent:
		eventsAttributesBoolKeys.Append(boolKeys)
		eventsAttributesBoolValues.Append(boolValues)
		eventsAttributesDoubleKeys.Append(doubleKeys)
		eventsAttributesDoubleValues.Append(doubleValues)
		eventsAttributesIntKeys.Append(intKeys)
		eventsAttributesIntValues.Append(intValues)
		eventsAttributesStrKeys.Append(strKeys)
		eventsAttributesStrValues.Append(strValues)
		eventsAttributesBytesKeys.Append(byteKeys)
		eventsAttributesBytesValue.Append(bytesValues)
	case AttributeTypeLink:
		linksAttributesBoolKeys.Append(boolKeys)
		linksAttributesBoolValues.Append(boolValues)
		linksAttributesDoubleKeys.Append(doubleKeys)
		linksAttributesDoubleValues.Append(doubleValues)
		linksAttributesIntKeys.Append(intKeys)
		linksAttributesIntValues.Append(intValues)
		linksAttributesStrKeys.Append(strKeys)
		linksAttributesStrValues.Append(strValues)
		linksAttributesBytesKeys.Append(byteKeys)
		linksAttributesBytesValue.Append(bytesValues)
	}
}

// AppendGroup Writes a complete pcommon.Map to the database. AttributesGroup and pcommon.Map have a one-to-one relationship.
func AppendGroup(attributeType AttributeType, group AttributesGroup) {
	switch attributeType {
	case AttributeTypeResource:
		resourceAttributesBoolKey.Append(group.BoolKeys)
		resourceAttributesBoolValue.Append(group.BoolValues)
		resourceAttributesDoubleKey.Append(group.DoubleKeys)
		resourceAttributesDoubleValue.Append(group.DoubleValues)
		resourceAttributesIntKey.Append(group.IntKeys)
		resourceAttributesIntValue.Append(group.IntValues)
		resourceAttributesStrKey.Append(group.StrKeys)
		resourceAttributesStrValue.Append(group.StrValues)
		resourceAttributesBytesKey.Append(group.BytesKeys)
		resourceAttributesBytesValue.Append(group.BytesValues)
	case AttributeTypeScope:
		scopeAttributesBoolKey.Append(group.BoolKeys)
		scopeAttributesBoolValue.Append(group.BoolValues)
		scopeAttributesDoubleKey.Append(group.DoubleKeys)
		scopeAttributesDoubleValue.Append(group.DoubleValues)
		scopeAttributesIntKey.Append(group.IntKeys)
		scopeAttributesIntValue.Append(group.IntValues)
		scopeAttributesStrKey.Append(group.StrKeys)
		scopeAttributesStrValue.Append(group.StrValues)
		scopeAttributesBytesKey.Append(group.BytesKeys)
		scopeAttributesBytesValue.Append(group.BytesValues)
	case AttributeTypeSpan:
		spanAttributesBoolKey.Append(group.BoolKeys)
		spanAttributesBoolValue.Append(group.BoolValues)
		spanAttributesDoubleKey.Append(group.DoubleKeys)
		spanAttributesDoubleValue.Append(group.DoubleValues)
		spanAttributesIntKey.Append(group.IntKeys)
		spanAttributesIntValue.Append(group.IntValues)
		spanAttributesStrKey.Append(group.StrKeys)
		spanAttributesStrValue.Append(group.StrValues)
		spanAttributesBytesKey.Append(group.BytesKeys)
		spanAttributesBytesValue.Append(group.BytesValues)
	}
}
