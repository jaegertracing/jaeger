// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/json"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

const (
	ValueTypeBool   = pcommon.ValueTypeBool
	ValueTypeDouble = pcommon.ValueTypeDouble
	ValueTypeInt    = pcommon.ValueTypeInt
	ValueTypeStr    = pcommon.ValueTypeStr
	ValueTypeBytes  = pcommon.ValueTypeBytes
	ValueTypeSlice  = pcommon.ValueTypeSlice
	ValueTypeMap    = pcommon.ValueTypeMap
)

// AttributesGroup holds categorized attributes as native Go slices for ClickHouse insertion.
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

// attributesToGroup Categorizes and aggregates Attributes based on the data type of their values.
func attributesToGroup(attributes pcommon.Map) AttributesGroup {
	var group AttributesGroup
	attributes.Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case ValueTypeBool:
			group.BoolKeys = append(group.BoolKeys, k)
			group.BoolValues = append(group.BoolValues, v.Bool())
		case ValueTypeDouble:
			group.DoubleKeys = append(group.DoubleKeys, k)
			group.DoubleValues = append(group.DoubleValues, v.Double())
		case ValueTypeInt:
			group.IntKeys = append(group.IntKeys, k)
			group.IntValues = append(group.IntValues, v.Int())
		case ValueTypeStr:
			group.StrKeys = append(group.StrKeys, k)
			group.StrValues = append(group.StrValues, v.Str())
		case ValueTypeBytes:
			group.BytesKeys = append(group.BytesKeys, k)
			group.BytesValues = append(group.BytesValues, base64.StdEncoding.EncodeToString(v.Bytes().AsRaw()))
		case ValueTypeSlice, ValueTypeMap:
			// For complex types, serialize to JSON string
			data, err := json.Marshal(convertValueToInterface(v))
			if err != nil {
				// Fallback: use string representation if JSON marshaling fails
				data, _ = json.Marshal(v.AsString())
			}
			group.StrKeys = append(group.StrKeys, k)
			group.StrValues = append(group.StrValues, string(data))
		default:
			// Handle other types as generic strings or ignore
			group.StrKeys = append(group.StrKeys, k)
			group.StrValues = append(group.StrValues, v.AsString())
		}
		return true
	})
	return group
}

// convertValueToInterface recursively converts pcommon.Value to a Go any
// suitable for JSON marshaling.
func convertValueToInterface(v pcommon.Value) any {
	switch v.Type() {
	case ValueTypeBool:
		return v.Bool()
	case ValueTypeDouble:
		return v.Double()
	case ValueTypeInt:
		return v.Int()
	case ValueTypeStr:
		return v.Str()
	case ValueTypeBytes:
		return base64.StdEncoding.EncodeToString(v.Bytes().AsRaw())
	case ValueTypeSlice:
		slice := make([]any, 0, v.Slice().Len())
		for i := 0; i < v.Slice().Len(); i++ {
			slice = append(slice, convertValueToInterface(v.Slice().At(i)))
		}
		return slice
	case ValueTypeMap:
		m := make(map[string]any, v.Map().Len())
		v.Map().Range(func(k string, mv pcommon.Value) bool {
			m[k] = convertValueToInterface(mv)
			return true
		})
		return m
	default:
		return v.AsString()
	}
}

// CombineAttributes merges resource, scope, and span attributes.
// Span attributes take precedence over scope, which take precedence over resource.
func CombineAttributes(resource, scope, span AttributesGroup) AttributesGroup {
	result := AttributesGroup{}

	// Add resource attributes
	result.BoolKeys = append(result.BoolKeys, resource.BoolKeys...)
	result.BoolValues = append(result.BoolValues, resource.BoolValues...)
	result.DoubleKeys = append(result.DoubleKeys, resource.DoubleKeys...)
	result.DoubleValues = append(result.DoubleValues, resource.DoubleValues...)
	result.IntKeys = append(result.IntKeys, resource.IntKeys...)
	result.IntValues = append(result.IntValues, resource.IntValues...)
	result.StrKeys = append(result.StrKeys, resource.StrKeys...)
	result.StrValues = append(result.StrValues, resource.StrValues...)
	result.BytesKeys = append(result.BytesKeys, resource.BytesKeys...)
	result.BytesValues = append(result.BytesValues, resource.BytesValues...)

	// Add scope attributes
	result.BoolKeys = append(result.BoolKeys, scope.BoolKeys...)
	result.BoolValues = append(result.BoolValues, scope.BoolValues...)
	result.DoubleKeys = append(result.DoubleKeys, scope.DoubleKeys...)
	result.DoubleValues = append(result.DoubleValues, scope.DoubleValues...)
	result.IntKeys = append(result.IntKeys, scope.IntKeys...)
	result.IntValues = append(result.IntValues, scope.IntValues...)
	result.StrKeys = append(result.StrKeys, scope.StrKeys...)
	result.StrValues = append(result.StrValues, scope.StrValues...)
	result.BytesKeys = append(result.BytesKeys, scope.BytesKeys...)
	result.BytesValues = append(result.BytesValues, scope.BytesValues...)

	// Add span attributes (last, so they take precedence)
	result.BoolKeys = append(result.BoolKeys, span.BoolKeys...)
	result.BoolValues = append(result.BoolValues, span.BoolValues...)
	result.DoubleKeys = append(result.DoubleKeys, span.DoubleKeys...)
	result.DoubleValues = append(result.DoubleValues, span.DoubleValues...)
	result.IntKeys = append(result.IntKeys, span.IntKeys...)
	result.IntValues = append(result.IntValues, span.IntValues...)
	result.StrKeys = append(result.StrKeys, span.StrKeys...)
	result.StrValues = append(result.StrValues, span.StrValues...)
	result.BytesKeys = append(result.BytesKeys, span.BytesKeys...)
	result.BytesValues = append(result.BytesValues, span.BytesValues...)

	return result
}

// ExtractAttributes is a wrapper around attributesToGroup.
func ExtractAttributes(attributes pcommon.Map) AttributesGroup {
	return attributesToGroup(attributes)
}
