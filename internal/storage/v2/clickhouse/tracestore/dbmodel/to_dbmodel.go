// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "go.opentelemetry.io/collector/pdata/pcommon"

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
	result := make(map[pcommon.ValueType]map[string]string, 8)
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
