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
	if !tracesNeedUTF8Sanitization(traces) {
		return traces
	}
	if traces.IsReadOnly() {
		cloned := ptrace.NewTraces()
		traces.CopyTo(cloned)
		traces = cloned
	}
	sanitizeTraces(traces)
	return traces
}

// tracesNeedUTF8Sanitization and sanitizeTraces walk the same fields in the
// same order. Keep them together: a string that is detected has to be
// repaired, and a string that is repaired has to be detected.
func tracesNeedUTF8Sanitization(traces ptrace.Traces) bool {
	for _, resourceSpans := range traces.ResourceSpans().All() {
		if !utf8.ValidString(resourceSpans.SchemaUrl()) ||
			attributesNeedUTF8Sanitization(resourceSpans.Resource().Attributes()) {
			return true
		}
		for _, scopeSpans := range resourceSpans.ScopeSpans().All() {
			scope := scopeSpans.Scope()
			if !utf8.ValidString(scopeSpans.SchemaUrl()) ||
				!utf8.ValidString(scope.Name()) ||
				!utf8.ValidString(scope.Version()) ||
				attributesNeedUTF8Sanitization(scope.Attributes()) {
				return true
			}
			for _, span := range scopeSpans.Spans().All() {
				if spanNeedsUTF8Sanitization(span) {
					return true
				}
			}
		}
	}
	return false
}

func spanNeedsUTF8Sanitization(span ptrace.Span) bool {
	if !utf8.ValidString(span.Name()) ||
		!utf8.ValidString(span.TraceState().AsRaw()) ||
		!utf8.ValidString(span.Status().Message()) ||
		attributesNeedUTF8Sanitization(span.Attributes()) {
		return true
	}
	for _, event := range span.Events().All() {
		if !utf8.ValidString(event.Name()) || attributesNeedUTF8Sanitization(event.Attributes()) {
			return true
		}
	}
	for _, link := range span.Links().All() {
		if !utf8.ValidString(link.TraceState().AsRaw()) || attributesNeedUTF8Sanitization(link.Attributes()) {
			return true
		}
	}
	return false
}

// attributesNeedUTF8Sanitization and sanitizeValue both switch on the value
// kind. Extend both when a new kind can hold a string.
func attributesNeedUTF8Sanitization(attributes pcommon.Map) bool {
	for k, v := range attributes.All() {
		if !utf8.ValidString(k) || valueNeedsUTF8Sanitization(v) {
			return true
		}
	}
	return false
}

func valueNeedsUTF8Sanitization(v pcommon.Value) bool {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return !utf8.ValidString(v.Str())
	case pcommon.ValueTypeMap:
		return attributesNeedUTF8Sanitization(v.Map())
	case pcommon.ValueTypeSlice:
		for _, elem := range v.Slice().All() {
			if valueNeedsUTF8Sanitization(elem) {
				return true
			}
		}
	default:
	}
	return false
}

func sanitizeTraces(traces ptrace.Traces) {
	for _, resourceSpans := range traces.ResourceSpans().All() {
		if schema := resourceSpans.SchemaUrl(); !utf8.ValidString(schema) {
			resourceSpans.SetSchemaUrl(strings.ToValidUTF8(schema, utf8Replacement))
		}
		sanitizeAttributes(resourceSpans.Resource().Attributes())

		for _, scopeSpans := range resourceSpans.ScopeSpans().All() {
			if schema := scopeSpans.SchemaUrl(); !utf8.ValidString(schema) {
				scopeSpans.SetSchemaUrl(strings.ToValidUTF8(schema, utf8Replacement))
			}
			scope := scopeSpans.Scope()
			if name := scope.Name(); !utf8.ValidString(name) {
				scope.SetName(strings.ToValidUTF8(name, utf8Replacement))
			}
			if version := scope.Version(); !utf8.ValidString(version) {
				scope.SetVersion(strings.ToValidUTF8(version, utf8Replacement))
			}
			sanitizeAttributes(scope.Attributes())

			for _, span := range scopeSpans.Spans().All() {
				sanitizeSpan(span)
			}
		}
	}
}

func sanitizeSpan(span ptrace.Span) {
	if name := span.Name(); !utf8.ValidString(name) {
		span.SetName(strings.ToValidUTF8(name, utf8Replacement))
	}
	if state := span.TraceState().AsRaw(); !utf8.ValidString(state) {
		span.TraceState().FromRaw("")
	}
	status := span.Status()
	if message := status.Message(); !utf8.ValidString(message) {
		status.SetMessage(strings.ToValidUTF8(message, utf8Replacement))
	}
	sanitizeAttributes(span.Attributes())

	for _, event := range span.Events().All() {
		if name := event.Name(); !utf8.ValidString(name) {
			event.SetName(strings.ToValidUTF8(name, utf8Replacement))
		}
		sanitizeAttributes(event.Attributes())
	}
	for _, link := range span.Links().All() {
		if state := link.TraceState().AsRaw(); !utf8.ValidString(state) {
			link.TraceState().FromRaw("")
		}
		sanitizeAttributes(link.Attributes())
	}
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
