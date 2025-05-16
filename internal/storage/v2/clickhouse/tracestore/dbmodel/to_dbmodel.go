// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/hex"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	ValueTypeBool   = pcommon.ValueTypeBool
	ValueTypeDouble = pcommon.ValueTypeDouble
	ValueTypeInt    = pcommon.ValueTypeInt
	ValueTypeStr    = pcommon.ValueTypeStr
	ValueTypeBytes  = pcommon.ValueTypeBytes
)

// ToDBModel Converts the OTel pipeline Traces into a slice of Clickhouse domain model for batch insertion.
func ToDBModel(td ptrace.Traces) []Trace {
	var traces []Trace
	for _, resourceSpan := range td.ResourceSpans().All() {
		rs := resourceSpan.Resource()
		resource := Resource{
			Attributes: attributesToGroup(rs.Attributes()),
		}
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			sc := scopeSpan.Scope()
			scope := Scope{
				Name:       sc.Name(),
				Version:    sc.Version(),
				Attributes: attributesToGroup(sc.Attributes()),
			}
			for _, sp := range scopeSpan.Spans().All() {
				span := Span{
					Timestamp:     sp.StartTimestamp().AsTime(),
					TraceId:       encodeTraceID(sp.TraceID()),
					SpanId:        encodeSpanID(sp.SpanID()),
					ParentSpanId:  encodeSpanID(sp.ParentSpanID()),
					TraceState:    sp.TraceState().AsRaw(),
					Name:          sp.Name(),
					Kind:          sp.Kind().String(),
					Duration:      sp.EndTimestamp().AsTime(),
					StatusCode:    sp.Status().Code().String(),
					StatusMessage: sp.Status().Message(),
					Attributes:    attributesToGroup(sp.Attributes()),
				}
				trace := Trace{
					Resource: resource,
					Scope:    scope,
					Span:     span,
				}
				if sp.Events().Len() > 0 {
					trace.Events = make([]Event, sp.Events().Len())
				}
				if sp.Links().Len() > 0 {
					trace.Links = make([]Link, sp.Links().Len())
				}

				for i, e := range sp.Events().All() {
					event := Event{
						Name:       e.Name(),
						Timestamp:  e.Timestamp().AsTime(),
						Attributes: attributesToGroup(e.Attributes()),
					}
					trace.Events[i] = event
				}
				for i, l := range sp.Links().All() {
					link := Link{
						TraceId:    encodeTraceID(l.TraceID()),
						SpanId:     encodeSpanID(l.SpanID()),
						TraceState: l.TraceState().AsRaw(),
						Attributes: attributesToGroup(l.Attributes()),
					}
					trace.Links[i] = link
				}
				traces = append(traces, trace)
			}
		}
	}

	return traces
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
				group.BytesValues = append(group.BytesValues, v.Bytes().AsRaw())
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

// encodeTraceID convert pcommon.TraceID to [16]byte.
func encodeTraceID(id pcommon.TraceID) []byte {
	if id.IsEmpty() {
		return nil
	}
	bytes, _ := hex.DecodeString(id.String())
	return bytes
}

// encodeSpanID convert pcommon.SpanID to [8]byte.
func encodeSpanID(id pcommon.SpanID) []byte {
	if id.IsEmpty() {
		return nil
	}
	bytes, _ := hex.DecodeString(id.String())
	return bytes
}
