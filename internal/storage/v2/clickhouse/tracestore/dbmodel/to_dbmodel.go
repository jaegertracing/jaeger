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
// ptrace.SpanEventSlice and ptrace.SpanLinkSlice are stored in a Nested format in the database.
// Since all arrays in Nested need to have the same length, AttributesGroup cannot be used directly.
type NestedAttributesGroup struct {
	AttributesGroups []AttributesGroup
}

// AttributesGroup captures all data from a single pcommon.Map, except
// complex attributes (like slice or map) which are currently not supported.
// AttributesGroup consists of pairs of vectors for each of the supported primitive
// types, e.g. (BoolKeys, BoolValues). Every attribute in the pcommon.Map is mapped
// to one of the pairs depending on its type. The slices in each pair have identical
// length, which may be different from length in another pair. For example, if the
// pcommon.Map has no Boolean attributes then (BoolKeys=[], BoolValues=[]).
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

// AttributeColumnPair maps Attribute/Attributes to table columns. Instead of directly storing the entire Attribute/Attributes into a single independent Column,
// it splits them based on the value type.
// Assuming the value type here is string (since the key is always string, there's no need to consider it separately).
// For resource/scope/span attributes, keyCol/valueCol respectively contain all string-typed keys and values from the attribute, which can be seen as array(string).
// For events/links attributes, the situation is more complex because a span can have multiple events/links. Therefore, keyCol/valueCol will contain all key/value pairs from all events/links, which can be seen as array(array(string)).
type AttributeColumnPair struct {
	keyColName   string
	keyCol       proto.Column
	valueColName string
	valueCol     proto.Column
}

type AttributeColumnsMap map[pcommon.ValueType]AttributeColumnPair

var AllAttributeColumns = make(map[AttributeType]AttributeColumnsMap)

// init function is automatically executed during program startup to initialize global Attribute column information.
func init() {
	addAttributeTypeColumns := func(attributeType AttributeType, valueType pcommon.ValueType, keyCol proto.Column, valueCol proto.Column) {
		if _, ok := AllAttributeColumns[attributeType]; !ok {
			AllAttributeColumns[attributeType] = make(AttributeColumnsMap)
		}
		AllAttributeColumns[attributeType][valueType] = AttributeColumnPair{
			keyColName:   attributeType.String() + "Attributes" + valueType.String() + "Key",
			keyCol:       keyCol,
			valueColName: attributeType.String() + "Attributes" + valueType.String() + "Value",
			valueCol:     valueCol,
		}
	}

	buildColumnsForAttributeType := func(attributeType AttributeType) {
		if attributeType == AttributeTypeEvent || attributeType == AttributeTypeLink {
			addAttributeTypeColumns(attributeType, ValueTypeBool,
				proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
				proto.NewArray(new(proto.ColBool).Array()))
			addAttributeTypeColumns(attributeType, ValueTypeDouble,
				proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
				proto.NewArray(new(proto.ColFloat64).LowCardinality().Array()))
			addAttributeTypeColumns(attributeType, ValueTypeInt,
				proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
				proto.NewArray(new(proto.ColInt64).LowCardinality().Array()))
			addAttributeTypeColumns(attributeType, ValueTypeStr,
				proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
				proto.NewArray(new(proto.ColStr).LowCardinality().Array()))
			addAttributeTypeColumns(attributeType, ValueTypeBytes,
				proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
				proto.NewArray(new(proto.ColStr).LowCardinality().Array()))
		} else {
			addAttributeTypeColumns(attributeType, ValueTypeBool, new(proto.ColStr).LowCardinality().Array(), new(proto.ColBool).Array())
			addAttributeTypeColumns(attributeType, ValueTypeDouble, new(proto.ColStr).LowCardinality().Array(), new(proto.ColFloat64).LowCardinality().Array())
			addAttributeTypeColumns(attributeType, ValueTypeInt, new(proto.ColStr).LowCardinality().Array(), new(proto.ColInt64).LowCardinality().Array())
			addAttributeTypeColumns(attributeType, ValueTypeStr, new(proto.ColStr).LowCardinality().Array(), new(proto.ColStr).LowCardinality().Array())
			addAttributeTypeColumns(attributeType, ValueTypeBytes, new(proto.ColStr).LowCardinality().Array(), new(proto.ColStr).LowCardinality().Array())
		}
	}

	attributeTypes := []AttributeType{
		AttributeTypeResource,
		AttributeTypeScope,
		AttributeTypeSpan,
		AttributeTypeEvent,
		AttributeTypeLink,
	}
	for _, attrType := range attributeTypes {
		buildColumnsForAttributeType(attrType)
	}
}

type AttributeType int32

const (
	AttributeTypeResource AttributeType = iota
	AttributeTypeScope
	AttributeTypeSpan
	AttributeTypeEvent
	AttributeTypeLink
)

func (at AttributeType) String() string {
	switch at {
	case AttributeTypeResource:
		return "Resource"
	case AttributeTypeScope:
		return "Scope"
	case AttributeTypeSpan:
		return "Span"
	case AttributeTypeEvent:
		return "Event"
	case AttributeTypeLink:
		return "Link"
	default:
		return "Unknown"
	}
}

// ToDBModel Converts the OTel pipeline Traces into a ClickHouse-compatible format for batch insertion.
// It maps the trace attributes, spans, links and events from the OTel model to the appropriate ClickHouse column types
func ToDBModel(td ptrace.Traces) proto.Input {
	for i := range td.ResourceSpans().Len() {
		resourceSpans := td.ResourceSpans().At(i)
		resourceGroup := AttributesToGroup(resourceSpans.Resource().Attributes())

		for j := range resourceSpans.ScopeSpans().Len() {
			scope := resourceSpans.ScopeSpans().At(j).Scope()
			scopeGroup := AttributesToGroup(scope.Attributes())

			spans := resourceSpans.ScopeSpans().At(j).Spans()
			for k := range spans.Len() {
				span := spans.At(k)
				spanGroup := AttributesToGroup(span.Attributes())
				appendAttributeGroupToColumns(AttributeTypeResource, resourceGroup)
				appendAttributeGroupToColumns(AttributeTypeScope, scopeGroup)
				appendAttributeGroupToColumns(AttributeTypeSpan, spanGroup)

				var eventNestedGroup NestedAttributesGroup
				for l := range span.Events().Len() {
					event := span.Events().At(l)
					eventGroup := AttributesToGroup(event.Attributes())
					eventNestedGroup.AttributesGroups = append(eventNestedGroup.AttributesGroups, eventGroup)
				}
				appendNestedAttributeGroupToColumns(AttributeTypeEvent, eventNestedGroup)

				var linkNestedGroup NestedAttributesGroup
				for l := range span.Links().Len() {
					link := span.Links().At(l)
					linkGroup := AttributesToGroup(link.Attributes())
					linkNestedGroup.AttributesGroups = append(linkNestedGroup.AttributesGroups, linkGroup)
				}
				appendNestedAttributeGroupToColumns(AttributeTypeLink, linkNestedGroup)
			}
		}
	}
	attributeTypes := []AttributeType{
		AttributeTypeResource,
		AttributeTypeScope,
		AttributeTypeSpan,
		AttributeTypeEvent,
		AttributeTypeLink,
	}

	input := proto.Input{}
	for _, attrType := range attributeTypes {
		input = append(input, buildAttributesInput(attrType)...)
	}

	return input
}

func buildAttributesInput(attributeType AttributeType) proto.Input {
	var result []proto.InputColumn
	attrColumnsMap := AllAttributeColumns[attributeType]
	for _, pair := range attrColumnsMap {
		result = append(result, proto.InputColumn{
			Name: pair.keyColName,
			Data: pair.keyCol,
		})
		result = append(result, proto.InputColumn{
			Name: pair.valueColName,
			Data: pair.valueCol,
		})
	}
	return result
}

const (
	ValueTypeBool   = pcommon.ValueTypeBool
	ValueTypeDouble = pcommon.ValueTypeDouble
	ValueTypeInt    = pcommon.ValueTypeInt
	ValueTypeStr    = pcommon.ValueTypeStr
	ValueTypeBytes  = pcommon.ValueTypeBytes
)

// AppendNestedAttributeGroupToColumns Writes a complete set of pcommon.Map to the database. NestedAttributesGroup and pcommon.Map have a one-to-many relationship.
func appendNestedAttributeGroupToColumns(attributeType AttributeType, nestedGroup NestedAttributesGroup) {
	var boolKeys [][]string
	var boolValues [][]bool
	var doubleKeys [][]string
	var doubleValues [][]float64
	var intKeys [][]string
	var intValues [][]int64
	var strKeys [][]string
	var strValues [][]string
	var bytesKeys [][]string
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
		bytesKeys = append(bytesKeys, group.BytesKeys)
		bytesValues = append(bytesValues, group.BytesValues)
	}

	attrsColumnsMap := AllAttributeColumns[attributeType]

	boolKeyCol := attrsColumnsMap[ValueTypeBool].keyCol
	boolKeyCol.(*proto.ColArr[[]string]).Append(boolKeys)
	boolValueCol := attrsColumnsMap[ValueTypeBool].valueCol
	boolValueCol.(*proto.ColArr[[]bool]).Append(boolValues)

	doubleKeyCol := attrsColumnsMap[ValueTypeDouble].keyCol
	doubleKeyCol.(*proto.ColArr[[]string]).Append(doubleKeys)
	doubleValueCol := attrsColumnsMap[ValueTypeDouble].valueCol
	doubleValueCol.(*proto.ColArr[[]float64]).Append(doubleValues)

	intKeyCol := attrsColumnsMap[ValueTypeInt].keyCol
	intKeyCol.(*proto.ColArr[[]string]).Append(intKeys)
	intValueCol := attrsColumnsMap[ValueTypeInt].valueCol
	intValueCol.(*proto.ColArr[[]int64]).Append(intValues)

	strKeyCol := attrsColumnsMap[ValueTypeStr].keyCol
	strKeyCol.(*proto.ColArr[[]string]).Append(strKeys)
	strValueCol := attrsColumnsMap[ValueTypeStr].valueCol
	strValueCol.(*proto.ColArr[[]string]).Append(strValues)

	bytesKeyCol := attrsColumnsMap[ValueTypeBytes].keyCol
	bytesKeyCol.(*proto.ColArr[[]string]).Append(bytesKeys)
	bytesValueCol := attrsColumnsMap[ValueTypeBytes].valueCol
	bytesValueCol.(*proto.ColArr[[]string]).Append(bytesValues)
}

// AppendGroup Writes a complete pcommon.Map to the database. AttributesGroup and pcommon.Map have a one-to-one relationship.
func appendAttributeGroupToColumns(attributeType AttributeType, group AttributesGroup) {
	attrColumnsMap := AllAttributeColumns[attributeType]

	boolKeyCol := attrColumnsMap[ValueTypeBool].keyCol
	boolKeyCol.(*proto.ColArr[string]).Append(group.BoolKeys)
	boolValueCol := attrColumnsMap[ValueTypeBool].valueCol
	boolValueCol.(*proto.ColArr[bool]).Append(group.BoolValues)

	doubleKeyCol := attrColumnsMap[ValueTypeDouble].keyCol
	doubleKeyCol.(*proto.ColArr[string]).Append(group.DoubleKeys)
	doubleValueCol := attrColumnsMap[ValueTypeDouble].valueCol
	doubleValueCol.(*proto.ColArr[float64]).Append(group.DoubleValues)

	intKeyCol := attrColumnsMap[ValueTypeInt].keyCol
	intKeyCol.(*proto.ColArr[string]).Append(group.IntKeys)
	intValueCol := attrColumnsMap[ValueTypeInt].valueCol
	intValueCol.(*proto.ColArr[int64]).Append(group.IntValues)

	strKeyCol := attrColumnsMap[ValueTypeStr].keyCol
	strKeyCol.(*proto.ColArr[string]).Append(group.StrKeys)
	strValueCol := attrColumnsMap[ValueTypeStr].valueCol
	strValueCol.(*proto.ColArr[string]).Append(group.StrValues)

	bytesKeyCol := attrColumnsMap[ValueTypeBytes].keyCol
	bytesKeyCol.(*proto.ColArr[string]).Append(group.BytesKeys)
	bytesValueCol := attrColumnsMap[ValueTypeBytes].valueCol
	bytesValueCol.(*proto.ColArr[string]).Append(group.BytesValues)
}
