// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/json"
	"math"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type Trace struct {
	Resource Resource
	Scope    Scope
	Span     Span
	Links    []Link
	Events   []Event
}

type Resource struct {
	Attributes AttributesGroup
}

type Scope struct {
	Attributes AttributesGroup
}

type Span struct {
	Attributes AttributesGroup
}

type Link struct {
	Attributes AttributesGroup
}

type Event struct {
	Attributes AttributesGroup
}

func ToPTrace(dbTrace Trace) ptrace.Traces {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()

	resourceAttributes := AttributesGroupToMap(dbTrace.Resource.Attributes)
	resourceAttributes.CopyTo(resourceSpans.Resource().Attributes())

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	scopeAttributes := AttributesGroupToMap(dbTrace.Scope.Attributes)
	scopeAttributes.CopyTo(scopeSpans.Scope().Attributes())

	spans := scopeSpans.Spans().AppendEmpty()
	spanAttributes := AttributesGroupToMap(dbTrace.Span.Attributes)
	spanAttributes.CopyTo(spans.Attributes())

	for i := 0; i < len(dbTrace.Events); i++ {
		event := spans.Events().AppendEmpty()
		eventAttributes := AttributesGroupToMap(dbTrace.Events[i].Attributes)
		eventAttributes.CopyTo(event.Attributes())
	}

	for i := 0; i < len(dbTrace.Links); i++ {
		link := spans.Links().AppendEmpty()
		linkAttributes := AttributesGroupToMap(dbTrace.Links[i].Attributes)
		linkAttributes.CopyTo(link.Attributes())
	}
	return trace
}

func AttributesGroupToMap(group AttributesGroup) pcommon.Map {
	result := pcommon.NewMap()

	for i := 0; i < len(group.BytesKeys); i++ {
		key := group.BytesKeys[i]
		value := group.BytesValues[i]
		bts, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			panic(err)
		}
		result.PutEmptyBytes(key).FromRaw(bts)
	}

	for i := 0; i < len(group.MapKeys); i++ {
		key := group.MapKeys[i]
		jsonStr := group.MapValues[i]

		var dataMap map[string]any
		// TODO ptrace.JSONMarshaler maybe helpful
		err := json.Unmarshal([]byte(jsonStr), &dataMap)
		if err != nil {
			panic(err)
		}
		err = result.PutEmptyMap(key).FromRaw(dataMap)
		if err != nil {
			return pcommon.Map{}
		}
		for _, v := range dataMap {
			// FIXME data loss when value like: 3.0
			if value, ok := v.(float64); ok {
				int64Map := make(map[string]any, len(dataMap))
				if isInteger(value) {
					for dk, dv := range dataMap {
						int64Map[dk] = int64(dv.(float64))
					}
					result.Remove(key)
					err = result.PutEmptyMap(key).FromRaw(int64Map)
					if err != nil {
						return pcommon.Map{}
					}
					break
				}
			}
		}
	}

	for i := 0; i < len(group.SliceKeys); i++ {
		key := group.SliceKeys[i]
		value := group.SliceValues[i]

		var dataSlice []any
		err := json.Unmarshal([]byte(value), &dataSlice)
		if err != nil {
			panic(err)
		}
		err = result.PutEmptySlice(key).FromRaw(dataSlice)
		if err != nil {
			return pcommon.Map{}
		}

		for _, v := range dataSlice {
			if value, ok := v.(float64); ok {
				// FIXME data loss when value like: 3.0
				if isInteger(value) {
					int64Slice := make([]any, len(dataSlice))
					for di, dv := range dataSlice {
						int64Slice[di] = int64(dv.(float64))
					}
					result.Remove(key)
					err = result.PutEmptySlice(key).FromRaw(int64Slice)
					if err != nil {
						return pcommon.Map{}
					}
					break
				}
			}
		}
	}
	return result
}

func isInteger(f float64) bool {
	_, frac := math.Modf(f)
	return frac == 0
}
