// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// utf8Replacement is the U+FFFD character strings.ToValidUTF8 writes over an
// invalid byte sequence.
const utf8Replacement = "\uFFFD"

// NewUTF8Sanitizer returns a sanitizer that rewrites invalid UTF-8 in trace
// data so a strict protobuf decoder can accept the batch.
//
// String fields and attribute keys are rewritten in place with U+FFFD.
// Invalid trace state is cleared, because a placeholder is not valid W3C
// tracestate. A repaired key that matches a key already in the map is
// dropped, so the existing valid attribute is kept. When several invalid
// keys repair to the same string, the first in map iteration order is kept.
// pdata maps iterate in insertion order. Valid traces are returned unchanged.
func NewUTF8Sanitizer() Func {
	return sanitizeUTF8
}

func sanitizeUTF8(traces ptrace.Traces) ptrace.Traces {
	for _, resourceSpans := range traces.ResourceSpans().All() {
		if schema := resourceSpans.SchemaUrl(); !utf8.ValidString(schema) {
			if traces.IsReadOnly() {
				return sanitizeUTF8(cloneTraces(traces))
			}
			resourceSpans.SetSchemaUrl(strings.ToValidUTF8(schema, utf8Replacement))
		}
		if next, restart := sanitizeAttributesReadOnly(traces, resourceSpans.Resource().Attributes()); restart {
			return sanitizeUTF8(next)
		}

		for _, scopeSpans := range resourceSpans.ScopeSpans().All() {
			if schema := scopeSpans.SchemaUrl(); !utf8.ValidString(schema) {
				if traces.IsReadOnly() {
					return sanitizeUTF8(cloneTraces(traces))
				}
				scopeSpans.SetSchemaUrl(strings.ToValidUTF8(schema, utf8Replacement))
			}
			scope := scopeSpans.Scope()
			if name := scope.Name(); !utf8.ValidString(name) {
				if traces.IsReadOnly() {
					return sanitizeUTF8(cloneTraces(traces))
				}
				scope.SetName(strings.ToValidUTF8(name, utf8Replacement))
			}
			if version := scope.Version(); !utf8.ValidString(version) {
				if traces.IsReadOnly() {
					return sanitizeUTF8(cloneTraces(traces))
				}
				scope.SetVersion(strings.ToValidUTF8(version, utf8Replacement))
			}
			if next, restart := sanitizeAttributesReadOnly(traces, scope.Attributes()); restart {
				return sanitizeUTF8(next)
			}

			for _, span := range scopeSpans.Spans().All() {
				if next, restart := sanitizeSpan(traces, span); restart {
					return sanitizeUTF8(next)
				}
			}
		}
	}
	return traces
}

func sanitizeSpan(traces ptrace.Traces, span ptrace.Span) (ptrace.Traces, bool) {
	if name := span.Name(); !utf8.ValidString(name) {
		if traces.IsReadOnly() {
			return cloneTraces(traces), true
		}
		span.SetName(strings.ToValidUTF8(name, utf8Replacement))
	}
	if state := span.TraceState().AsRaw(); !utf8.ValidString(state) {
		if traces.IsReadOnly() {
			return cloneTraces(traces), true
		}
		span.TraceState().FromRaw("")
	}
	status := span.Status()
	if message := status.Message(); !utf8.ValidString(message) {
		if traces.IsReadOnly() {
			return cloneTraces(traces), true
		}
		status.SetMessage(strings.ToValidUTF8(message, utf8Replacement))
	}
	if next, restart := sanitizeAttributesReadOnly(traces, span.Attributes()); restart {
		return next, true
	}

	for _, event := range span.Events().All() {
		if name := event.Name(); !utf8.ValidString(name) {
			if traces.IsReadOnly() {
				return cloneTraces(traces), true
			}
			event.SetName(strings.ToValidUTF8(name, utf8Replacement))
		}
		if next, restart := sanitizeAttributesReadOnly(traces, event.Attributes()); restart {
			return next, true
		}
	}
	for _, link := range span.Links().All() {
		if state := link.TraceState().AsRaw(); !utf8.ValidString(state) {
			if traces.IsReadOnly() {
				return cloneTraces(traces), true
			}
			link.TraceState().FromRaw("")
		}
		if next, restart := sanitizeAttributesReadOnly(traces, link.Attributes()); restart {
			return next, true
		}
	}
	return traces, false
}

// sanitizeAttributesReadOnly clones traces before a read-only attribute map is
// written. The caller restarts the walk on the clone. A mutable map is
// sanitized here.
func sanitizeAttributesReadOnly(traces ptrace.Traces, attributes pcommon.Map) (ptrace.Traces, bool) {
	if traces.IsReadOnly() {
		if attributesHaveInvalidUTF8(attributes) {
			return cloneTraces(traces), true
		}
		return traces, false
	}
	sanitizeAttributes(attributes)
	return traces, false
}

func cloneTraces(traces ptrace.Traces) ptrace.Traces {
	cloned := ptrace.NewTraces()
	traces.CopyTo(cloned)
	return cloned
}

func attributesHaveInvalidUTF8(attributes pcommon.Map) bool {
	for k, v := range attributes.All() {
		if !utf8.ValidString(k) || valueHasInvalidUTF8(v) {
			return true
		}
	}
	return false
}

func valueHasInvalidUTF8(v pcommon.Value) bool {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return !utf8.ValidString(v.Str())
	case pcommon.ValueTypeMap:
		return attributesHaveInvalidUTF8(v.Map())
	case pcommon.ValueTypeSlice:
		for _, elem := range v.Slice().All() {
			if valueHasInvalidUTF8(elem) {
				return true
			}
		}
	default:
	}
	return false
}

func sanitizeAttributes(attributes pcommon.Map) {
	// Keys are collected before they are removed so the map is not mutated
	// while ranging. Range order is pdata's insertion order.
	type invalidKey struct {
		key   string
		value pcommon.Value
	}
	var invalidKeys []invalidKey
	attributes.Range(func(k string, v pcommon.Value) bool {
		sanitizeValue(v)
		if !utf8.ValidString(k) {
			moved := pcommon.NewValueEmpty()
			v.CopyTo(moved)
			invalidKeys = append(invalidKeys, invalidKey{key: k, value: moved})
		}
		return true
	})
	for _, item := range invalidKeys {
		attributes.Remove(item.key)
	}
	// A repaired key that is already present belongs to a valid attribute, or
	// to an earlier invalid key. Keep that one and drop this repair.
	for _, item := range invalidKeys {
		repaired := strings.ToValidUTF8(item.key, utf8Replacement)
		if _, exists := attributes.Get(repaired); exists {
			continue
		}
		item.value.CopyTo(attributes.PutEmpty(repaired))
	}
}

func sanitizeValue(v pcommon.Value) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		if value := v.Str(); !utf8.ValidString(value) {
			v.SetStr(strings.ToValidUTF8(value, utf8Replacement))
		}
	case pcommon.ValueTypeMap:
		sanitizeAttributes(v.Map())
	case pcommon.ValueTypeSlice:
		for _, elem := range v.Slice().All() {
			sanitizeValue(elem)
		}
	default:
	}
}
