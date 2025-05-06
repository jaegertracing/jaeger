// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/ch-go/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// ToDBModel Converts the OTel pipeline Traces into a ClickHouse-compatible format for batch insertion.
// It maps the trace attributes, spans, links and events from the OTel model to the appropriate ClickHouse column types
func ToDBModel(td ptrace.Traces) proto.Input {
	traceColumnSet := TraceColumnSet{}
	traceColumnSet.init()
	for _, resourceSpan := range td.ResourceSpans().All() {
		resourceGroup := attributesToGroup(resourceSpan.Resource().Attributes())

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scope := scopeSpan.Scope()
			scopeGroup := attributesToGroup(scope.Attributes())

			for _, span := range scopeSpan.Spans().All() {
				spanGroup := attributesToGroup(span.Attributes())

				timestampCol := traceColumnSet.span.timestamp.Col
				timestampCol.(*proto.ColDateTime64).Append(span.StartTimestamp().AsTime())
				traceIDCol := traceColumnSet.span.traceID.Col
				traceIDCol.(*proto.ColLowCardinality[string]).Append(traceIDToHexString(span.TraceID()))
				spanIDCol := traceColumnSet.span.spanID.Col
				spanIDCol.(*proto.ColLowCardinality[string]).Append(spanIDToHexString(span.SpanID()))
				parentSpanIDCol := traceColumnSet.span.parentSpanID.Col
				parentSpanIDCol.(*proto.ColLowCardinality[string]).Append(spanIDToHexString(span.ParentSpanID()))
				traceStateCol := traceColumnSet.span.traceState.Col
				traceStateCol.(*proto.ColLowCardinality[string]).Append(span.TraceState().AsRaw())
				spanNameCol := traceColumnSet.span.name.Col
				spanNameCol.(*proto.ColLowCardinality[string]).Append(span.Name())
				spanKindCol := traceColumnSet.span.kind.Col
				spanKindCol.(*proto.ColLowCardinality[string]).Append(span.Kind().String())
				scopeNameCol := traceColumnSet.scope.name.Col
				scopeNameCol.(*proto.ColLowCardinality[string]).Append(scope.Name())
				scopeVersion := traceColumnSet.scope.version.Col
				scopeVersion.(*proto.ColLowCardinality[string]).Append(scope.Version())
				durationCol := traceColumnSet.span.duration.Col
				durationCol.(*proto.ColDateTime64).Append(span.EndTimestamp().AsTime())
				statusCodeCol := traceColumnSet.span.statusCode.Col
				statusCodeCol.(*proto.ColLowCardinality[string]).Append(span.Status().Code().String())
				statusMessageCol := traceColumnSet.span.statusMessage.Col
				statusMessageCol.(*proto.ColLowCardinality[string]).Append(span.Status().Message())

				var eventsName []string
				var eventsTimestamp []time.Time
				var eventNestedGroup NestedAttributesGroup
				for _, event := range span.Events().All() {
					eventsName = append(eventsName, event.Name())
					eventsTimestamp = append(eventsTimestamp, event.Timestamp().AsTime())
					eventGroup := attributesToGroup(event.Attributes())
					eventNestedGroup.AttributesGroups = append(eventNestedGroup.AttributesGroups, eventGroup)
				}
				eventsTimestampCol := traceColumnSet.events.timestamps.Col
				eventsTimestampCol.(*proto.ColArr[time.Time]).Append(eventsTimestamp)
				eventsNameCol := traceColumnSet.events.names.Col
				eventsNameCol.(*proto.ColArr[string]).Append(eventsName)

				var linksTraceId []string
				var linksSpanId []string
				var linksTracesState []string
				var linkNestedGroup NestedAttributesGroup
				for _, link := range span.Links().All() {
					linksTraceId = append(linksTraceId, traceIDToHexString(link.TraceID()))
					linksSpanId = append(linksSpanId, spanIDToHexString(link.SpanID()))
					linksTracesState = append(linksTracesState, link.TraceState().AsRaw())
					linkGroup := attributesToGroup(link.Attributes())
					linkNestedGroup.AttributesGroups = append(linkNestedGroup.AttributesGroups, linkGroup)
				}
				linksSpanIdCol := traceColumnSet.links.spanID.Col
				linksSpanIdCol.(*proto.ColArr[string]).Append(linksSpanId)
				linksTraceIdCol := traceColumnSet.links.traceID.Col
				linksTraceIdCol.(*proto.ColArr[string]).Append(linksTraceId)
				linksTraceStateCol := traceColumnSet.links.traceState.Col
				linksTraceStateCol.(*proto.ColArr[string]).Append(linksTracesState)

				traceColumnSet.resource.attributes.appendAttributeGroup(resourceGroup)
				traceColumnSet.scope.attributes.appendAttributeGroup(scopeGroup)
				traceColumnSet.span.attributes.appendAttributeGroup(spanGroup)
				traceColumnSet.events.attributes.appendNestedAttributeGroup(eventNestedGroup)
				traceColumnSet.links.attributes.appendNestedAttributeGroup(linkNestedGroup)
			}
		}
	}

	input := proto.Input{}
	input = append(input, traceColumnSet.span.spanInput()...)
	input = append(input, traceColumnSet.scope.scopeInput()...)
	input = append(input, traceColumnSet.resource.resourceInput()...)
	input = append(input, traceColumnSet.events.eventsInput()...)
	input = append(input, traceColumnSet.links.linkInput()...)
	return input
}

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

// attributesToGroup Categorizes and aggregates Attributes based on the data type of their values, and writes them in batches.
func attributesToGroup(attributes pcommon.Map) AttributesGroup {
	attributesMap := attributesToMap(attributes)
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

// attributesToMap Groups a pcommon.Map by data type and splits the key-value pairs into arrays for storage.
// The values in the key-value pairs of a pcommon.Map instance may not all be of the same data type.
// For example, a pcommon.Map can contain key-value pairs such as:
// string-string, string-bool, string-int64, string-float64. Clearly, the key-value pairs need to be classified based on the data type.
func attributesToMap(attrs pcommon.Map) map[pcommon.ValueType]map[string]pcommon.Value {
	result := make(map[pcommon.ValueType]map[string]pcommon.Value)
	for _, valueType := range []pcommon.ValueType{
		ValueTypeBool, ValueTypeDouble, ValueTypeInt, ValueTypeStr, ValueTypeBytes,
	} {
		result[valueType] = make(map[string]pcommon.Value)
	}

	attrs.Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeMap:
			jsonStr, err := marshalValueToJSON(v)
			if err == nil {
				result[ValueTypeStr][k] = pcommon.NewValueStr(jsonStr)
			}
		case pcommon.ValueTypeSlice:
			jsonStr, err := marshalValueToJSON(v)
			if err == nil {
				result[ValueTypeStr][k] = pcommon.NewValueStr(jsonStr)
			}
		default:
			typ := v.Type()
			if _, exists := result[typ]; exists {
				result[typ][k] = v
			}
		}
		return true
	})
	return result
}

func marshalValueToJSON(v pcommon.Value) (string, error) {
	var val interface{}
	switch v.Type() {
	case pcommon.ValueTypeMap:
		val = valueToInterface(v.Map())
	case pcommon.ValueTypeSlice:
		val = valueToInterface(v.Slice())
	default:
		return "", fmt.Errorf("unsupported type for JSON serialization: %s", v.Type())
	}

	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func valueToInterface(val interface{}) interface{} {
	switch v := val.(type) {
	case pcommon.Map:
		m := make(map[string]interface{})
		v.Range(func(k string, val pcommon.Value) bool {
			m[k] = pcommonValueToInterface(val)
			return true
		})
		return m
	case pcommon.Slice:
		s := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			s[i] = pcommonValueToInterface(v.At(i))
		}
		return s
	default:
		return val
	}
}

func pcommonValueToInterface(v pcommon.Value) interface{} {
	switch v.Type() {
	case pcommon.ValueTypeBool:
		return v.Bool()
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeStr:
		return v.Str()
	case pcommon.ValueTypeBytes:
		return v.Bytes().AsRaw()
	case pcommon.ValueTypeMap:
		return valueToInterface(v.Map())
	case pcommon.ValueTypeSlice:
		return valueToInterface(v.Slice())
	default:
		return nil
	}
}

// AttributeColumnPair maps Attribute/Attributes to table init. Instead of directly storing the entire Attribute/Attributes into a single independent Column,
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

type attributeColumnsMap map[pcommon.ValueType]AttributeColumnPair

type TraceColumnPair struct {
	ColName string
	Col     proto.Column
}

type TraceColumnSet struct {
	resource ResourceColumnSet
	scope    ScopeColumnSet
	span     SpanColumnSet
	events   EventColumnSet
	links    LinkColumnSet
}

type ResourceColumnSet struct {
	attributes attributeColumnsMap
}

type ScopeColumnSet struct {
	name       TraceColumnPair
	version    TraceColumnPair
	attributes attributeColumnsMap
}

type SpanColumnSet struct {
	timestamp     TraceColumnPair
	traceID       TraceColumnPair
	spanID        TraceColumnPair
	parentSpanID  TraceColumnPair
	traceState    TraceColumnPair
	name          TraceColumnPair
	kind          TraceColumnPair
	duration      TraceColumnPair
	statusCode    TraceColumnPair
	statusMessage TraceColumnPair
	attributes    attributeColumnsMap
}

type EventColumnSet struct {
	names      TraceColumnPair
	timestamps TraceColumnPair
	attributes attributeColumnsMap
}
type LinkColumnSet struct {
	traceID    TraceColumnPair
	spanID     TraceColumnPair
	traceState TraceColumnPair
	attributes attributeColumnsMap
}

func (ts *TraceColumnSet) init() {
	ts.resource = newResourceColumns()
	ts.scope = newScopeColumns()
	ts.span = newSpanColumns()
	ts.events = newEventsColumns()
	ts.links = newLinkColumns()
}

func newResourceColumns() ResourceColumnSet {
	attributes := attributeColumnsMap{}
	newAttributeColumns(&attributes, AttributeTypeResource)
	return ResourceColumnSet{
		attributes: attributes,
	}
}

func newScopeColumns() ScopeColumnSet {
	attributes := attributeColumnsMap{}
	newAttributeColumns(&attributes, AttributeTypeScope)
	return ScopeColumnSet{
		name:       newTraceColumnsPair("ScopeName", new(proto.ColStr).LowCardinality()),
		version:    newTraceColumnsPair("ScopeVersion", new(proto.ColStr).LowCardinality()),
		attributes: attributes,
	}
}

func newSpanColumns() SpanColumnSet {
	attributes := attributeColumnsMap{}
	newAttributeColumns(&attributes, AttributeTypeSpan)
	return SpanColumnSet{
		timestamp:     newTraceColumnsPair("Timestamp", new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano)),
		traceID:       newTraceColumnsPair("TraceID", new(proto.ColStr).LowCardinality()),
		spanID:        newTraceColumnsPair("SpanID", new(proto.ColStr).LowCardinality()),
		traceState:    newTraceColumnsPair("TraceState", new(proto.ColStr).LowCardinality()),
		parentSpanID:  newTraceColumnsPair("ParentSpanID", new(proto.ColStr).LowCardinality()),
		name:          newTraceColumnsPair("SpanName", new(proto.ColStr).LowCardinality()),
		kind:          newTraceColumnsPair("SpanKind", new(proto.ColStr).LowCardinality()),
		duration:      newTraceColumnsPair("Duration", new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano)),
		statusCode:    newTraceColumnsPair("StatusCode", new(proto.ColStr).LowCardinality()),
		statusMessage: newTraceColumnsPair("StatusMessage", new(proto.ColStr).LowCardinality()),
		attributes:    attributes,
	}
}

func newEventsColumns() EventColumnSet {
	attributes := attributeColumnsMap{}
	newAttributeColumns(&attributes, AttributeTypeEvent)
	return EventColumnSet{
		names:      newTraceColumnsPair("EventsName", new(proto.ColStr).LowCardinality().Array()),
		timestamps: newTraceColumnsPair("EventsTimestamp", new(proto.ColDateTime64).WithPrecision(proto.PrecisionNano).Array()),
		attributes: attributes,
	}
}

func newLinkColumns() LinkColumnSet {
	attributes := attributeColumnsMap{}
	newAttributeColumns(&attributes, AttributeTypeLink)
	return LinkColumnSet{
		traceID:    newTraceColumnsPair("LinksTraceId", new(proto.ColStr).LowCardinality().Array()),
		spanID:     newTraceColumnsPair("LinksSpanId", new(proto.ColStr).LowCardinality().Array()),
		traceState: newTraceColumnsPair("LinksTraceStatus", new(proto.ColStr).LowCardinality().Array()),
		attributes: attributes,
	}
}

func newTraceColumnsPair(colName string, col proto.Column) TraceColumnPair {
	return TraceColumnPair{
		ColName: colName,
		Col:     col,
	}
}

func newAttributeColumns(acm *attributeColumnsMap, attributeType AttributeType) {
	if attributeType == AttributeTypeEvent || attributeType == AttributeTypeLink {
		acm.buildAttrColumns(attributeType, ValueTypeBool,
			proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
			proto.NewArray(new(proto.ColBool).Array()))
		acm.buildAttrColumns(attributeType, ValueTypeDouble,
			proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
			proto.NewArray(new(proto.ColFloat64).Array()))
		acm.buildAttrColumns(attributeType, ValueTypeInt,
			proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
			proto.NewArray(new(proto.ColInt64).Array()))
		acm.buildAttrColumns(attributeType, ValueTypeStr,
			proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
			proto.NewArray(new(proto.ColStr).LowCardinality().Array()))
		acm.buildAttrColumns(attributeType, ValueTypeBytes,
			proto.NewArray(new(proto.ColStr).LowCardinality().Array()),
			proto.NewArray(new(proto.ColStr).LowCardinality().Array()))
	} else {
		acm.buildAttrColumns(attributeType, ValueTypeBool, new(proto.ColStr).LowCardinality().Array(), new(proto.ColBool).Array())
		acm.buildAttrColumns(attributeType, ValueTypeDouble, new(proto.ColStr).LowCardinality().Array(), new(proto.ColFloat64).Array())
		acm.buildAttrColumns(attributeType, ValueTypeInt, new(proto.ColStr).LowCardinality().Array(), new(proto.ColInt64).Array())
		acm.buildAttrColumns(attributeType, ValueTypeStr, new(proto.ColStr).LowCardinality().Array(), new(proto.ColStr).LowCardinality().Array())
		acm.buildAttrColumns(attributeType, ValueTypeBytes, new(proto.ColStr).LowCardinality().Array(), new(proto.ColStr).LowCardinality().Array())
	}
}

func (acm attributeColumnsMap) buildAttrColumns(attributeType AttributeType, valueType pcommon.ValueType, keyCol proto.Column, valueCol proto.Column) {
	acm[valueType] = AttributeColumnPair{
		keyColName:   attributeType.String() + "Attributes" + valueType.String() + "Key",
		keyCol:       keyCol,
		valueColName: attributeType.String() + "Attributes" + valueType.String() + "Value",
		valueCol:     valueCol,
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

func (rs *ResourceColumnSet) resourceInput() proto.Input {
	return rs.attributes.attributesInput()
}

func (sc *ScopeColumnSet) scopeInput() proto.Input {
	result := proto.Input{
		input(sc.name.ColName, sc.name.Col),
		input(sc.version.ColName, sc.version.Col),
	}

	result = append(result, sc.attributes.attributesInput()...)
	return result
}

func (s *SpanColumnSet) spanInput() proto.Input {
	result := proto.Input{
		input(s.timestamp.ColName, s.timestamp.Col),
		input(s.traceID.ColName, s.traceID.Col),
		input(s.spanID.ColName, s.spanID.Col),
		input(s.parentSpanID.ColName, s.parentSpanID.Col),
		input(s.traceState.ColName, s.traceState.Col),
		input(s.name.ColName, s.name.Col),
		input(s.kind.ColName, s.kind.Col),
		input(s.duration.ColName, s.duration.Col),
		input(s.statusCode.ColName, s.statusCode.Col),
		input(s.statusMessage.ColName, s.statusMessage.Col),
	}
	result = append(result, s.attributes.attributesInput()...)
	return result
}

func (event *EventColumnSet) eventsInput() proto.Input {
	result := proto.Input{
		input(event.names.ColName, event.names.Col),
		input(event.timestamps.ColName, event.timestamps.Col),
	}
	result = append(result, event.attributes.attributesInput()...)
	return result
}

func (link *LinkColumnSet) linkInput() proto.Input {
	result := proto.Input{
		input(link.traceID.ColName, link.traceID.Col),
		input(link.spanID.ColName, link.spanID.Col),
		input(link.traceState.ColName, link.traceState.Col),
	}

	result = append(result, link.attributes.attributesInput()...)
	return result
}

func (acm attributeColumnsMap) attributesInput() proto.Input {
	var result []proto.InputColumn
	for _, pair := range acm {
		result = append(result, input(pair.keyColName, pair.keyCol))
		result = append(result, input(pair.valueColName, pair.valueCol))
	}
	return result
}

func input(name string, data proto.ColInput) proto.InputColumn {
	return proto.InputColumn{
		Name: name,
		Data: data,
	}
}

const (
	ValueTypeBool   = pcommon.ValueTypeBool
	ValueTypeDouble = pcommon.ValueTypeDouble
	ValueTypeInt    = pcommon.ValueTypeInt
	ValueTypeStr    = pcommon.ValueTypeStr
	ValueTypeBytes  = pcommon.ValueTypeBytes
)

// appendNestedAttributeGroup Writes a complete set of pcommon.Map to the database. NestedAttributesGroup and pcommon.Map have a one-to-many relationship.
func (acm attributeColumnsMap) appendNestedAttributeGroup(nestedGroup NestedAttributesGroup) {
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

	boolKeyCol := acm[ValueTypeBool].keyCol
	boolKeyCol.(*proto.ColArr[[]string]).Append(boolKeys)
	boolValueCol := acm[ValueTypeBool].valueCol
	boolValueCol.(*proto.ColArr[[]bool]).Append(boolValues)

	doubleKeyCol := acm[ValueTypeDouble].keyCol
	doubleKeyCol.(*proto.ColArr[[]string]).Append(doubleKeys)
	doubleValueCol := acm[ValueTypeDouble].valueCol
	doubleValueCol.(*proto.ColArr[[]float64]).Append(doubleValues)

	intKeyCol := acm[ValueTypeInt].keyCol
	intKeyCol.(*proto.ColArr[[]string]).Append(intKeys)
	intValueCol := acm[ValueTypeInt].valueCol
	intValueCol.(*proto.ColArr[[]int64]).Append(intValues)

	strKeyCol := acm[ValueTypeStr].keyCol
	strKeyCol.(*proto.ColArr[[]string]).Append(strKeys)
	strValueCol := acm[ValueTypeStr].valueCol
	strValueCol.(*proto.ColArr[[]string]).Append(strValues)

	bytesKeyCol := acm[ValueTypeBytes].keyCol
	bytesKeyCol.(*proto.ColArr[[]string]).Append(bytesKeys)
	bytesValueCol := acm[ValueTypeBytes].valueCol
	bytesValueCol.(*proto.ColArr[[]string]).Append(bytesValues)
}

// appendAttributeGroup Writes a complete pcommon.Map to the database. AttributesGroup and pcommon.Map have a one-to-one relationship.
func (acm attributeColumnsMap) appendAttributeGroup(group AttributesGroup) {
	boolKeyCol := acm[pcommon.ValueTypeBool].keyCol
	boolKeyCol.(*proto.ColArr[string]).Append(group.BoolKeys)
	boolValueCol := acm[ValueTypeBool].valueCol
	boolValueCol.(*proto.ColArr[bool]).Append(group.BoolValues)

	doubleKeyCol := acm[ValueTypeDouble].keyCol
	doubleKeyCol.(*proto.ColArr[string]).Append(group.DoubleKeys)
	doubleValueCol := acm[ValueTypeDouble].valueCol
	doubleValueCol.(*proto.ColArr[float64]).Append(group.DoubleValues)

	intKeyCol := acm[ValueTypeInt].keyCol
	intKeyCol.(*proto.ColArr[string]).Append(group.IntKeys)
	intValueCol := acm[ValueTypeInt].valueCol
	intValueCol.(*proto.ColArr[int64]).Append(group.IntValues)

	strKeyCol := acm[ValueTypeStr].keyCol
	strKeyCol.(*proto.ColArr[string]).Append(group.StrKeys)
	strValueCol := acm[ValueTypeStr].valueCol
	strValueCol.(*proto.ColArr[string]).Append(group.StrValues)

	bytesKeyCol := acm[ValueTypeBytes].keyCol
	bytesKeyCol.(*proto.ColArr[string]).Append(group.BytesKeys)
	bytesValueCol := acm[ValueTypeBytes].valueCol
	bytesValueCol.(*proto.ColArr[string]).Append(group.BytesValues)
}

func traceIDToHexString(id pcommon.TraceID) string {
	return hex.EncodeToString(id[:])
}

func spanIDToHexString(id pcommon.SpanID) string {
	return hex.EncodeToString(id[:])
}
