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
// tracestate. Valid traces are returned unchanged.
func NewUTF8Sanitizer() Func {
	return sanitizeUTF8
}

func sanitizeUTF8(traces ptrace.Traces) ptrace.Traces {
	if !tracesNeedUTF8Sanitization(traces) {
		return traces
	}

	var workingTraces ptrace.Traces
	if traces.IsReadOnly() {
		workingTraces = ptrace.NewTraces()
		traces.CopyTo(workingTraces)
	} else {
		workingTraces = traces
	}

	for _, resourceSpans := range workingTraces.ResourceSpans().All() {
		resource := resourceSpans.Resource()
		sanitizeString(resourceSpans.SchemaUrl(), resourceSpans.SetSchemaUrl)
		sanitizeAttributes(resource.Attributes())

		for _, scopeSpans := range resourceSpans.ScopeSpans().All() {
			scope := scopeSpans.Scope()
			sanitizeString(scopeSpans.SchemaUrl(), scopeSpans.SetSchemaUrl)
			sanitizeString(scope.Name(), scope.SetName)
			sanitizeString(scope.Version(), scope.SetVersion)
			sanitizeAttributes(scope.Attributes())

			for _, span := range scopeSpans.Spans().All() {
				sanitizeSpan(span)
			}
		}
	}
	return workingTraces
}

func sanitizeSpan(span ptrace.Span) {
	sanitizeString(span.Name(), span.SetName)
	sanitizeTraceState(span.TraceState())
	status := span.Status()
	sanitizeString(status.Message(), status.SetMessage)
	sanitizeAttributes(span.Attributes())

	for _, event := range span.Events().All() {
		sanitizeString(event.Name(), event.SetName)
		sanitizeAttributes(event.Attributes())
	}
	for _, link := range span.Links().All() {
		sanitizeTraceState(link.TraceState())
		sanitizeAttributes(link.Attributes())
	}
}

func sanitizeString(value string, set func(string)) {
	if !utf8.ValidString(value) {
		set(strings.ToValidUTF8(value, utf8Replacement))
	}
}

func sanitizeTraceState(state pcommon.TraceState) {
	if !utf8.ValidString(state.AsRaw()) {
		state.FromRaw("")
	}
}

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

func sanitizeAttributes(attributes pcommon.Map) {
	// Keys are repaired after the scan so the map is not mutated while ranging.
	// The value moves onto the repaired key; a new diagnostic key could overwrite
	// an attribute that is already present.
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
		item.value.CopyTo(attributes.PutEmpty(strings.ToValidUTF8(item.key, utf8Replacement)))
	}
}

func sanitizeValue(v pcommon.Value) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		sanitizeString(v.Str(), v.SetStr)
	case pcommon.ValueTypeMap:
		sanitizeAttributes(v.Map())
	case pcommon.ValueTypeSlice:
		for _, elem := range v.Slice().All() {
			sanitizeValue(elem)
		}
	default:
	}
}
