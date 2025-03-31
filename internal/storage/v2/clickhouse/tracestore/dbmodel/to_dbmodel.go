// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	ValueTypeBool   = pcommon.ValueTypeBool
	ValueTypeDouble = pcommon.ValueTypeDouble
	ValueTypeInt    = pcommon.ValueTypeInt
	ValueTypeStr    = pcommon.ValueTypeStr
	ValueTypeEmpty  = pcommon.ValueTypeEmpty
	ValueTypeMap    = pcommon.ValueTypeMap
	ValueTypeSlice  = pcommon.ValueTypeSlice
	ValueTypeBytes  = pcommon.ValueTypeBytes
)

// AttributesGroup There is a one-to-one relationship between an AttributesGroup and a pcommon.Map.
// In the official ClickHouse implementation, pcommon.Map is stored as a string Map in the database for both key and value,
// which causes the loss of data types. See:
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/4930224211ea876c71367cf04038e23359c0338a/exporter/clickhouseexporter/exporter_traces.go#L162
// Therefore, splitting the key and value of pcommon.Map into arrays for storage ensures that 1. data types are not lost
// and 2. it can be more conveniently used as query parameters.
type AttributesGroup struct {
	BoolKeys       []string
	BoolValues     []bool
	DoubleKeys     []string
	DoubleValues   []float64
	IntKeys        []string
	IntValues      []int64
	StrKeys        []string
	StrValues      []string
	EmptyKeys      []string
	EmptyMapKeys   []string
	EmptySliceKeys []string
	EmptyBytesKeys []string
}

func toAttributesGroup(attributes map[pcommon.ValueType]map[string]any) AttributesGroup {
	var group AttributesGroup

	for key, val := range attributes[ValueTypeBool] {
		group.BoolKeys = append(group.BoolKeys, key)
		group.BoolValues = append(group.BoolValues, val.(bool))
	}

	for key, val := range attributes[ValueTypeDouble] {
		group.DoubleKeys = append(group.DoubleKeys, key)
		group.DoubleValues = append(group.DoubleValues, val.(float64))
	}

	for key, val := range attributes[ValueTypeInt] {
		group.IntKeys = append(group.IntKeys, key)
		group.IntValues = append(group.IntValues, val.(int64))
	}

	for key, val := range attributes[ValueTypeStr] {
		group.StrKeys = append(group.StrKeys, key)
		group.StrValues = append(group.StrValues, val.(string))
	}

	for key := range attributes[ValueTypeEmpty] {
		group.EmptyKeys = append(group.EmptyKeys, key)
	}

	for key := range attributes[ValueTypeMap] {
		group.EmptyMapKeys = append(group.EmptyMapKeys, key)
	}

	for key := range attributes[ValueTypeSlice] {
		group.EmptySliceKeys = append(group.EmptySliceKeys, key)
	}

	for key := range attributes[ValueTypeBytes] {
		group.EmptyBytesKeys = append(group.EmptyBytesKeys, key)
	}
	return group
}

// NestedAttributesGroup There is a one-to-many relationship between a NestedAttributesGroup and a pcommon.Map.
// In the official ClickHouse implementation, ptrace.SpanEventSlice and ptrace.SpanLinkSlice are stored in a Nested format in the database.
// Since all arrays in Nested need to have the same length, AttributesGroup cannot be used directly.
type NestedAttributesGroup struct {
	BoolKeys       [][]string
	BoolValues     [][]bool
	DoubleKeys     [][]string
	DoubleValues   [][]float64
	IntKeys        [][]string
	IntValues      [][]int64
	StrKeys        [][]string
	StrValues      [][]string
	EmptyKeys      [][]string
	EmptyMapKeys   [][]string
	EmptySliceKeys [][]string
	EmptyBytesKeys [][]string
}

// toAttributesMap Groups a pcommon.Map by data type and splits the key-value pairs into arrays for storage.
// The values in the key-value pairs of a pcommon.Map instance may not all be of the same data type.
// For example, a pcommon.Map can contain key-value pairs such as:
// string-string, string-bool, string-int64, string-float64. Clearly, the key-value pairs need to be classified based on the data type.
func toAttributesMap(attrs pcommon.Map) map[pcommon.ValueType]map[string]any {
	result := make(map[pcommon.ValueType]map[string]any, 8)
	// For all string-bool pairs, the result would look like: map[`identifier specific to bool`]map[string]bool
	for _, valueType := range []pcommon.ValueType{
		ValueTypeBool, ValueTypeDouble, ValueTypeInt, ValueTypeStr, ValueTypeEmpty,
		ValueTypeMap, ValueTypeSlice, ValueTypeBytes,
	} {
		result[valueType] = make(map[string]any)
	}
	// Fill according to the data type of the value
	attrs.Range(func(k string, v pcommon.Value) bool {
		valueType := v.Type()
		result[valueType][k] = v.AsRaw()
		return true
	})
	return result
}

type Events struct {
	timestamps []time.Time
	names      []string
	attributes NestedAttributesGroup
}

func fromEvents(events ptrace.SpanEventSlice) Events {
	times := make([]time.Time, 0, events.Len())
	names := make([]string, 0, events.Len())
	nestedAttrs := NestedAttributesGroup{
		BoolKeys:       make([][]string, 0, events.Len()),
		BoolValues:     make([][]bool, 0, events.Len()),
		DoubleKeys:     make([][]string, 0, events.Len()),
		DoubleValues:   make([][]float64, 0, events.Len()),
		IntKeys:        make([][]string, 0, events.Len()),
		IntValues:      make([][]int64, 0, events.Len()),
		StrKeys:        make([][]string, 0, events.Len()),
		StrValues:      make([][]string, 0, events.Len()),
		EmptyKeys:      make([][]string, 0, events.Len()),
		EmptyMapKeys:   make([][]string, 0, events.Len()),
		EmptySliceKeys: make([][]string, 0, events.Len()),
		EmptyBytesKeys: make([][]string, 0, events.Len()),
	}

	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		times = append(times, event.Timestamp().AsTime())
		names = append(names, event.Name())
		attrMap := toAttributesMap(event.Attributes())
		group := toAttributesGroup(attrMap)
		nestedAttrs.BoolKeys = append(nestedAttrs.BoolKeys, group.BoolKeys)
		nestedAttrs.BoolValues = append(nestedAttrs.BoolValues, group.BoolValues)
		nestedAttrs.DoubleKeys = append(nestedAttrs.DoubleKeys, group.DoubleKeys)
		nestedAttrs.DoubleValues = append(nestedAttrs.DoubleValues, group.DoubleValues)
		nestedAttrs.IntKeys = append(nestedAttrs.IntKeys, group.IntKeys)
		nestedAttrs.IntValues = append(nestedAttrs.IntValues, group.IntValues)
		nestedAttrs.StrKeys = append(nestedAttrs.StrKeys, group.StrKeys)
		nestedAttrs.StrValues = append(nestedAttrs.StrValues, group.StrValues)
		nestedAttrs.EmptyKeys = append(nestedAttrs.EmptyKeys, group.EmptyKeys)
		nestedAttrs.EmptyMapKeys = append(nestedAttrs.EmptyMapKeys, group.EmptyMapKeys)
		nestedAttrs.EmptySliceKeys = append(nestedAttrs.EmptySliceKeys, group.EmptySliceKeys)
		nestedAttrs.EmptyBytesKeys = append(nestedAttrs.EmptyBytesKeys, group.EmptyBytesKeys)
	}
	return Events{times, names, nestedAttrs}
}

type Links struct {
	traceIDs    []string
	spanIDs     []string
	traceStates []string
	attributes  NestedAttributesGroup
}

func fromLinks(links ptrace.SpanLinkSlice) Links {
	traceIDs := make([]string, 0, links.Len())
	spanIDs := make([]string, 0, links.Len())
	states := make([]string, 0, links.Len())
	nestedAttrs := NestedAttributesGroup{
		BoolKeys:       make([][]string, 0, links.Len()),
		BoolValues:     make([][]bool, 0, links.Len()),
		DoubleKeys:     make([][]string, 0, links.Len()),
		DoubleValues:   make([][]float64, 0, links.Len()),
		IntKeys:        make([][]string, 0, links.Len()),
		IntValues:      make([][]int64, 0, links.Len()),
		StrKeys:        make([][]string, 0, links.Len()),
		StrValues:      make([][]string, 0, links.Len()),
		EmptyKeys:      make([][]string, 0, links.Len()),
		EmptyMapKeys:   make([][]string, 0, links.Len()),
		EmptySliceKeys: make([][]string, 0, links.Len()),
		EmptyBytesKeys: make([][]string, 0, links.Len()),
	}

	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		traceIDs = append(traceIDs, traceIDToHexOrEmptyString(link.TraceID()))
		spanIDs = append(spanIDs, spanIDToHexOrEmptyString(link.SpanID()))
		states = append(states, link.TraceState().AsRaw())
		attrMap := toAttributesMap(link.Attributes())
		group := toAttributesGroup(attrMap)
		nestedAttrs.BoolKeys = append(nestedAttrs.BoolKeys, group.BoolKeys)
		nestedAttrs.BoolValues = append(nestedAttrs.BoolValues, group.BoolValues)
		nestedAttrs.DoubleKeys = append(nestedAttrs.DoubleKeys, group.DoubleKeys)
		nestedAttrs.DoubleValues = append(nestedAttrs.DoubleValues, group.DoubleValues)
		nestedAttrs.IntKeys = append(nestedAttrs.IntKeys, group.IntKeys)
		nestedAttrs.IntValues = append(nestedAttrs.IntValues, group.IntValues)
		nestedAttrs.StrKeys = append(nestedAttrs.StrKeys, group.StrKeys)
		nestedAttrs.StrValues = append(nestedAttrs.StrValues, group.StrValues)
		nestedAttrs.EmptyKeys = append(nestedAttrs.EmptyKeys, group.EmptyKeys)
		nestedAttrs.EmptyMapKeys = append(nestedAttrs.EmptyMapKeys, group.EmptyMapKeys)
		nestedAttrs.EmptySliceKeys = append(nestedAttrs.EmptySliceKeys, group.EmptySliceKeys)
		nestedAttrs.EmptyBytesKeys = append(nestedAttrs.EmptyBytesKeys, group.EmptyBytesKeys)
	}
	return Links{traceIDs, spanIDs, states, nestedAttrs}
}
