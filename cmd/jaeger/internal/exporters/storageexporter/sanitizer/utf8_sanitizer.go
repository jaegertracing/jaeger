// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"unicode/utf8"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	invalidOperation = "InvalidOperationName"
	invalidService   = "InvalidServiceName"
	invalidTagKey    = "InvalidTagKey"
	badUTF8Prefix    = "bad_utf8_"
)

// NewUTF8Sanitizer creates a UTF8 sanitizer.
func NewUTF8Sanitizer() SanitizeTraces {
	return sanitizeUF8
}

// sanitizeUTF8 sanitizes the UTF8 in the spans.
func sanitizeUF8(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				// Sanitize operation name
				if !utf8.ValidString(span.Name()) {
					originalName := span.Name()
					span.SetName(invalidOperation)
					byteSlice := span.Attributes().PutEmptyBytes(badUTF8Prefix + "operation_name")
					byteSlice.FromRaw([]byte(originalName))
				}

				// Sanitize service name attribute
				attributes := span.Attributes()
				serviceNameAttr, ok := attributes.Get("service.name")
				if ok && !utf8.ValidString(serviceNameAttr.Str()) {
					originalServiceName := serviceNameAttr.Str()
					attributes.PutStr("service.name", invalidService)
					byteSlice := attributes.PutEmptyBytes(badUTF8Prefix + "service.name")
					byteSlice.FromRaw([]byte(originalServiceName))
				}

				sanitizeAttributes(attributes)
			}
		}
	}
	return traces
}

// sanitizeAttributes sanitizes attributes to ensure UTF8 validity.
func sanitizeAttributes(attributes pcommon.Map) {
	attributes.Range(func(k string, v pcommon.Value) bool {
		// Handle invalid UTF8 in attribute keys
		if !utf8.ValidString(k) {
			originalKey := k
			attributes.PutStr(invalidTagKey, k)
			byteSlice := attributes.PutEmptyBytes(badUTF8Prefix + originalKey)
			byteSlice.FromRaw([]byte(originalKey))
		} else if v.Type() == pcommon.ValueTypeStr && !utf8.ValidString(v.Str()) {
			// Handle invalid UTF8 in attribute values
			originalValue := v.Str()
			attributes.PutStr(k, invalidTagKey)
			byteSlice := attributes.PutEmptyBytes(badUTF8Prefix + k)
			byteSlice.FromRaw([]byte(originalValue))
		}
		return true
	})
}
