// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/hex"

	"github.com/ClickHouse/ch-go/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// ToDBModel Converts the OTel pipeline Traces into a ClickHouse-compatible format for batch insertion.
// It maps the trace attributes, spans, links and events from the OTel model to the dbmodel span.
func ToDBModel(td ptrace.Traces) []Span {
	dbSpans := []Span{}
	for _, resourceSpan := range td.ResourceSpans().All() {
		// TODO: add attribute
		resourceAttrs := resourceSpan.Resource().Attributes()
		serviceName := ""
		if val, ok := resourceAttrs.Get("service.name"); ok {
			serviceName = val.Str()
		}
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scope := scopeSpan.Scope()
			for _, span := range scopeSpan.Spans().All() {
				traceID := traceIDToHexString(span.TraceID())
				spanID := spanIDToHexString(span.SpanID())
				parentID := spanIDToHexString(span.ParentSpanID())
				traceState := span.TraceState().AsRaw()
				spanName := span.Name()
				spanKind := span.Kind().String()
				scopeName := scope.Name()
				scopeVersion := scope.Version()
				duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
				startTime := span.StartTimestamp().AsTime()
				statusCode := span.Status().Code().String()
				statusMessage := span.Status().Message()

				events := []Event{}
				links := []Link{}

				// Process events
				var eventNestedGroup NestedAttributesGroup
				for _, event := range span.Events().All() {
					name := event.Name()
					tp := event.Timestamp().AsTime()
					dbmodel_event := Event{
						Name:      name,
						Timestamp: tp,
					}
					events = append(events, dbmodel_event)
					eventGroup := attributesToGroup(event.Attributes())
					eventNestedGroup.AttributesGroups = append(eventNestedGroup.AttributesGroups, eventGroup)
				}

				// Process links
				var linkNestedGroup NestedAttributesGroup
				for _, link := range span.Links().All() {
					linkGroup := attributesToGroup(link.Attributes())
					linkNestedGroup.AttributesGroups = append(linkNestedGroup.AttributesGroups, linkGroup)
					lTraceId := traceIDToHexString(link.TraceID())
					lSpanId := spanIDToHexString(link.SpanID())
					lTracesState := link.TraceState().AsRaw()
					link := Link{
						TraceID:    lTraceId,
						SpanID:     lSpanId,
						TraceState: lTracesState,
					}
					links = append(links, link)
				}

				// Construct Span and append into dbSpans
				dbSpan := Span{
					ID:            spanID,
					TraceID:       traceID,
					TraceState:    traceState,
					ParentSpanID:  parentID,
					Name:          spanName,
					Kind:          spanKind,
					StartTime:     startTime,
					StatusCode:    statusCode,
					StatusMessage: statusMessage,
					Duration:      duration,
					Events:        events,
					Links:         links,
					ServiceName:   serviceName,
					ScopeName:     scopeName,
					ScopeVersion:  scopeVersion,
				}
				dbSpans = append(dbSpans, dbSpan)
			}
		}
	}

	return dbSpans
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
	// Fill according to the data type of the value
	for k, v := range attrs.All() {
		typ := v.Type()
		// For basic data types (such as bool, uint64, and float64) we can make sure type safe.
		// TODO: For non-basic types (such as Map, Slice), they should be serialized and stored as OTLP/JSON strings
		result[typ][k] = v
	}
	return result
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
