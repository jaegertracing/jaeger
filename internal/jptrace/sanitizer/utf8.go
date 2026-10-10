// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"fmt"
	"unicode/utf8"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/xpdata/entity"
)

const (
	invalidSpanName      = "invalid-span-name"
	invalidTagKey        = "invalid-tag-key"
	invalidEventName     = "invalid-event-name"
	invalidStatusMessage = "invalid-status-message"
	invalidScopeName     = "invalid-scope-name"
	invalidScopeVersion  = "invalid-scope-version"
	invalidTraceState    = "invalid-trace-state"
	invalidSchemaURL     = "invalid-schema-url"
)

// NewUTF8Sanitizer returns a sanitizer function that makes every string in the trace
// data valid UTF-8, since a backend that validates UTF-8 rejects the whole batch:
//   - Attribute keys and string values of resources, scopes, spans, span events and
//     span links are checked, including those nested in maps and slices. An invalid
//     value is replaced with a bytes value holding the original data, and an invalid key
//     with an "invalid-tag-key-N" key whose bytes value holds the original key and value.
//   - An invalid span name, event name, scope name, scope version or status message is
//     replaced with a placeholder, and the original bytes are kept in an attribute of the
//     same span, event or scope, keyed by that placeholder.
//   - An invalid trace state or schema URL is cleared, and the original bytes are kept in
//     an "invalid-trace-state" or "invalid-schema-url" attribute of the owning span, link,
//     resource or scope.
//   - A resource entity ref holding an invalid string is dropped, since it has no
//     attributes to keep the original bytes in; the resource attributes it names are kept.
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

	for _, resourceSpan := range workingTraces.ResourceSpans().All() {
		resource := resourceSpan.Resource()
		sanitizeString(resourceSpan.SchemaUrl(), resourceSpan.SetSchemaUrl, "", resource.Attributes(), invalidSchemaURL)
		sanitizeAttributes(resource.Attributes())
		entity.ResourceEntityRefs(resource).RemoveIf(entityRefNeedsUTF8Sanitization)

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scope := scopeSpan.Scope()
			sanitizeString(scopeSpan.SchemaUrl(), scopeSpan.SetSchemaUrl, "", scope.Attributes(), invalidSchemaURL)
			sanitizeString(scope.Name(), scope.SetName, invalidScopeName, scope.Attributes(), invalidScopeName)
			sanitizeString(scope.Version(), scope.SetVersion, invalidScopeVersion, scope.Attributes(), invalidScopeVersion)
			sanitizeAttributes(scope.Attributes())

			for _, span := range scopeSpan.Spans().All() {
				sanitizeSpan(span)
			}
		}
	}

	return workingTraces
}

func sanitizeSpan(span ptrace.Span) {
	attributes := span.Attributes()
	sanitizeString(span.Name(), span.SetName, invalidSpanName, attributes, invalidSpanName)
	sanitizeString(span.TraceState().AsRaw(), span.TraceState().FromRaw, "", attributes, invalidTraceState)
	status := span.Status()
	sanitizeString(status.Message(), status.SetMessage, invalidStatusMessage, attributes, invalidStatusMessage)
	sanitizeAttributes(attributes)

	for _, event := range span.Events().All() {
		sanitizeString(event.Name(), event.SetName, invalidEventName, event.Attributes(), invalidEventName)
		sanitizeAttributes(event.Attributes())
	}

	for _, link := range span.Links().All() {
		sanitizeString(link.TraceState().AsRaw(), link.TraceState().FromRaw, "", link.Attributes(), invalidTraceState)
		sanitizeAttributes(link.Attributes())
	}
}

// sanitizeString replaces an invalid value through set, keeping the original bytes
// in attributes under key.
func sanitizeString(value string, set func(string), replacement string, attributes pcommon.Map, key string) {
	if utf8.ValidString(value) {
		return
	}
	attributes.PutEmptyBytes(key).FromRaw([]byte(value))
	set(replacement)
}

func tracesNeedUTF8Sanitization(traces ptrace.Traces) bool {
	for _, resourceSpan := range traces.ResourceSpans().All() {
		resource := resourceSpan.Resource()
		if !utf8.ValidString(resourceSpan.SchemaUrl()) || attributesNeedUTF8Sanitization(resource.Attributes()) {
			return true
		}
		for _, ref := range entity.ResourceEntityRefs(resource).All() {
			if entityRefNeedsUTF8Sanitization(ref) {
				return true
			}
		}

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scope := scopeSpan.Scope()
			if !utf8.ValidString(scopeSpan.SchemaUrl()) ||
				!utf8.ValidString(scope.Name()) ||
				!utf8.ValidString(scope.Version()) ||
				attributesNeedUTF8Sanitization(scope.Attributes()) {
				return true
			}

			for _, span := range scopeSpan.Spans().All() {
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

func entityRefNeedsUTF8Sanitization(ref entity.EntityRef) bool {
	return !utf8.ValidString(ref.SchemaUrl()) ||
		!utf8.ValidString(ref.Type()) ||
		stringsNeedUTF8Sanitization(ref.IdKeys()) ||
		stringsNeedUTF8Sanitization(ref.DescriptionKeys())
}

func stringsNeedUTF8Sanitization(values pcommon.StringSlice) bool {
	for _, s := range values.All() {
		if !utf8.ValidString(s) {
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
	// A decoded map can hold the same key more than once, so every invalid entry
	// is captured during iteration and all of them are removed in one pass.
	var invalidEntries [][]byte

	attributes.Range(func(k string, v pcommon.Value) bool {
		sanitizeValue(v)
		if !utf8.ValidString(k) {
			sanitized := []byte(k + ":")
			switch v.Type() {
			case pcommon.ValueTypeBytes:
				sanitized = append(sanitized, v.Bytes().AsRaw()...)
			default:
				sanitized = append(sanitized, v.AsString()...)
			}
			invalidEntries = append(invalidEntries, sanitized)
		}
		return true
	})
	if len(invalidEntries) == 0 {
		return
	}

	attributes.RemoveIf(func(k string, _ pcommon.Value) bool {
		return !utf8.ValidString(k)
	})
	for i, sanitized := range invalidEntries {
		attributes.PutEmptyBytes(fmt.Sprintf("%s-%d", invalidTagKey, i+1)).FromRaw(sanitized)
	}
}

func sanitizeValue(v pcommon.Value) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		if s := v.Str(); !utf8.ValidString(s) {
			v.SetEmptyBytes().FromRaw([]byte(s))
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
