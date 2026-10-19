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
	invalidSpanName        = "invalid-span-name"
	invalidTagKey          = "invalid-tag-key"
	invalidEventName       = "invalid-event-name"
	invalidStatusMessage   = "invalid-status-message"
	invalidScopeName       = "invalid-scope-name"
	invalidScopeVersion    = "invalid-scope-version"
	invalidTraceState      = "invalid-trace-state"
	invalidSchemaURL       = "invalid-schema-url"
	invalidEntitySchemaURL = "invalid-entity-schema-url"
	invalidEntityType      = "invalid-entity-type"
	invalidEntityIDKey     = "invalid-entity-id-key"
	invalidEntityDescKey   = "invalid-entity-description-key"
)

// NewUTF8Sanitizer returns a sanitizer that rewrites invalid UTF-8 in trace
// data. A backend that checks UTF-8 rejects the whole batch otherwise.
//
// Attribute keys and string values are sanitized on resources, scopes, spans,
// span events, and span links, including values nested in maps and slices. An
// invalid string value is replaced with a bytes value holding the original
// bytes. An invalid key is removed and recorded as "invalid-tag-key-N", whose
// bytes value holds the original key and value. Every invalid key is recorded,
// so a repeated key is not collapsed into one entry.
//
// Any other invalid string is replaced with a placeholder, and the original
// bytes are kept on the owning resource, scope, span, event, or link. That
// covers span and event names, scope name and version, status messages, trace
// state, schema URLs, and resource entity refs. Repeated entity-ref keys keep
// each original value under a numbered attribute.
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
		sanitizeResourceSpans(resourceSpans)
	}
	return workingTraces
}

func sanitizeResourceSpans(resourceSpans ptrace.ResourceSpans) {
	resource := resourceSpans.Resource()
	sanitizeString(resourceSpans.SchemaUrl(), resourceSpans.SetSchemaUrl, invalidSchemaURL, resource.Attributes())
	sanitizeAttributes(resource.Attributes())
	sanitizeEntityRefs(resource)

	for _, scopeSpans := range resourceSpans.ScopeSpans().All() {
		sanitizeScopeSpans(scopeSpans)
	}
}

func sanitizeScopeSpans(scopeSpans ptrace.ScopeSpans) {
	scope := scopeSpans.Scope()
	attributes := scope.Attributes()
	sanitizeString(scopeSpans.SchemaUrl(), scopeSpans.SetSchemaUrl, invalidSchemaURL, attributes)
	sanitizeString(scope.Name(), scope.SetName, invalidScopeName, attributes)
	sanitizeString(scope.Version(), scope.SetVersion, invalidScopeVersion, attributes)
	sanitizeAttributes(attributes)

	for _, span := range scopeSpans.Spans().All() {
		sanitizeSpan(span)
	}
}

func sanitizeSpan(span ptrace.Span) {
	attributes := span.Attributes()
	sanitizeString(span.Name(), span.SetName, invalidSpanName, attributes)
	sanitizeString(span.TraceState().AsRaw(), span.TraceState().FromRaw, invalidTraceState, attributes)
	status := span.Status()
	sanitizeString(status.Message(), status.SetMessage, invalidStatusMessage, attributes)
	sanitizeAttributes(attributes)

	for _, event := range span.Events().All() {
		sanitizeString(event.Name(), event.SetName, invalidEventName, event.Attributes())
		sanitizeAttributes(event.Attributes())
	}
	for _, link := range span.Links().All() {
		sanitizeString(link.TraceState().AsRaw(), link.TraceState().FromRaw, invalidTraceState, link.Attributes())
		sanitizeAttributes(link.Attributes())
	}
}

func sanitizeEntityRefs(resource pcommon.Resource) {
	attributes := resource.Attributes()
	idKey := 1
	descriptionKey := 1
	for _, ref := range entity.ResourceEntityRefs(resource).All() {
		sanitizeString(ref.SchemaUrl(), ref.SetSchemaUrl, invalidEntitySchemaURL, attributes)
		sanitizeString(ref.Type(), ref.SetType, invalidEntityType, attributes)
		idKey = sanitizeStringSlice(ref.IdKeys(), invalidEntityIDKey, attributes, idKey)
		descriptionKey = sanitizeStringSlice(ref.DescriptionKeys(), invalidEntityDescKey, attributes, descriptionKey)
	}
}

// sanitizeString replaces an invalid value with placeholder and keeps the
// original bytes in attributes under that same key.
func sanitizeString(value string, set func(string), placeholder string, attributes pcommon.Map) {
	if utf8.ValidString(value) {
		return
	}
	attributes.PutEmptyBytes(placeholder).FromRaw([]byte(value))
	set(placeholder)
}

// sanitizeStringSlice rewrites each invalid element in place. n numbers the
// attribute that keeps the original bytes, so repeated keys are all recorded.
func sanitizeStringSlice(values pcommon.StringSlice, placeholder string, attributes pcommon.Map, n int) int {
	for i := 0; i < values.Len(); i++ {
		value := values.At(i)
		if utf8.ValidString(value) {
			continue
		}
		attributes.PutEmptyBytes(fmt.Sprintf("%s-%d", placeholder, n)).FromRaw([]byte(value))
		values.SetAt(i, placeholder)
		n++
	}
	return n
}

func tracesNeedUTF8Sanitization(traces ptrace.Traces) bool {
	for _, resourceSpans := range traces.ResourceSpans().All() {
		resource := resourceSpans.Resource()
		if !utf8.ValidString(resourceSpans.SchemaUrl()) ||
			attributesNeedUTF8Sanitization(resource.Attributes()) ||
			entityRefsNeedUTF8Sanitization(resource) {
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

func entityRefsNeedUTF8Sanitization(resource pcommon.Resource) bool {
	for _, ref := range entity.ResourceEntityRefs(resource).All() {
		if !utf8.ValidString(ref.SchemaUrl()) ||
			!utf8.ValidString(ref.Type()) ||
			stringsNeedUTF8Sanitization(ref.IdKeys()) ||
			stringsNeedUTF8Sanitization(ref.DescriptionKeys()) {
			return true
		}
	}
	return false
}

func stringsNeedUTF8Sanitization(values pcommon.StringSlice) bool {
	for _, value := range values.All() {
		if !utf8.ValidString(value) {
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
	// A decoded map can repeat a key. A Go map would keep only one of them.
	var invalidKeys [][]byte
	attributes.Range(func(k string, v pcommon.Value) bool {
		sanitizeValue(v)
		if !utf8.ValidString(k) {
			invalidKeys = append(invalidKeys, attributeKeyPayload(k, v))
		}
		return true
	})
	if len(invalidKeys) == 0 {
		return
	}

	attributes.RemoveIf(func(k string, _ pcommon.Value) bool {
		return !utf8.ValidString(k)
	})
	for i, payload := range invalidKeys {
		attributes.PutEmptyBytes(fmt.Sprintf("%s-%d", invalidTagKey, i+1)).FromRaw(payload)
	}
}

func attributeKeyPayload(key string, value pcommon.Value) []byte {
	sanitized := []byte(key + ":")
	switch value.Type() {
	case pcommon.ValueTypeBytes:
		sanitized = append(sanitized, value.Bytes().AsRaw()...)
	default:
		sanitized = append(sanitized, value.AsString()...)
	}
	return sanitized
}

func sanitizeValue(v pcommon.Value) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		if value := v.Str(); !utf8.ValidString(value) {
			v.SetEmptyBytes().FromRaw([]byte(value))
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
