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
	invalidTagKey    = "InvalidTagKey"
	badUTF8Prefix    = "bad_utf8_"
)

// NewUTF8Sanitizer creates a UTF8 sanitizer.
func NewUTF8Sanitizer() SanitizeTraces {
	return sanitizeUTF8
}

// sanitizeUTF8 sanitizes the UTF8 in the traces.
func sanitizeUTF8(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)

		// Sanitize resource attributes
		sanitizeAttributes(resourceSpan.Resource().Attributes())

		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			scopeSpan := scopeSpans.At(j)

			// Sanitize scope attributes
			sanitizeAttributes(scopeSpan.Scope().Attributes())

			spans := scopeSpan.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)

				// Sanitize operation name
				if !utf8.ValidString(span.Name()) {
					originalName := span.Name()
					span.SetName(invalidOperation)
					binaryAttr := span.Attributes().PutEmptyBytes(badUTF8Prefix + "operation_name")
					binaryAttr.FromRaw([]byte(originalName))
				}

				// Sanitize span attributes
				sanitizeAttributes(span.Attributes())
			}
		}
	}
	return traces
}

// sanitizeAttributes sanitizes attributes to ensure UTF8 validity.
func sanitizeAttributes(attributes pcommon.Map) {
	// Collect invalid keys and values during iteration
	var invalidKeys []string
	invalidValues := make(map[string]string)

	attributes.Range(func(k string, v pcommon.Value) bool {
		// Handle invalid UTF-8 in attribute keys
		if !utf8.ValidString(k) {
			invalidKeys = append(invalidKeys, k)
		}
		// Handle invalid UTF-8 in attribute values
		if v.Type() == pcommon.ValueTypeStr && !utf8.ValidString(v.Str()) {
			invalidValues[k] = v.Str()
		}
		return true
	})

	// Apply collected changes after iteration
	for _, k := range invalidKeys {
		originalKey := k
		attributes.PutStr(invalidTagKey, k)
		binaryAttr := attributes.PutEmptyBytes(badUTF8Prefix + originalKey)
		binaryAttr.FromRaw([]byte(originalKey))
	}

	for k, v := range invalidValues {
		originalValue := v
		attributes.PutStr(k, invalidTagKey)
		binaryAttr := attributes.PutEmptyBytes(badUTF8Prefix + k)
		binaryAttr.FromRaw([]byte(originalValue))
	}
}
