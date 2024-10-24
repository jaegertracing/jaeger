// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"fmt"
	"unicode/utf8"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	invalidSpanName = "invalid-span-name"
	invalidTagKey   = "invalid-tag-key"
)

// NewUTF8Sanitizer returns a sanitizer function that performs the following sanitizations
// on the trace data:
//   - Checks all attributes of the span to ensure that the keys are valid UTF-8 strings
//     and all string typed values are valid UTF-8 strings. If any keys or values are invalid,
//     they are replaced with a valid UTF-8 string containing debugging data related to the invalidations.
//   - Explicitly checks that all span names are valid UTF-8 strings. If they are not, they are replaced
//     with a valid UTF-8 string and debugging information is put into the attributes of the span.
func NewUTF8Sanitizer() Func {
	return sanitizeUTF8
}

func sanitizeUTF8(traces ptrace.Traces) ptrace.Traces {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		sanitizeAttributes(resourceSpan.Resource().Attributes())

		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			scopeSpan := scopeSpans.At(j)
			sanitizeAttributes(scopeSpan.Scope().Attributes())

			spans := scopeSpan.Spans()

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)

				if !utf8.ValidString(span.Name()) {
					sanitized := []byte(span.Name())
					newVal := span.Attributes().PutEmptyBytes(invalidSpanName)
					newVal.Append(sanitized...)
					span.SetName(invalidSpanName)
				}

				sanitizeAttributes(span.Attributes())
			}
		}
	}

	return traces
}

func sanitizeAttributes(attributes pcommon.Map) {
	// collect invalid keys during iteration to avoid changing the keys of the map
	// while iterating over the map using Range
	invalidKeys := make(map[string]pcommon.Value)

	attributes.Range(func(k string, v pcommon.Value) bool {
		if !utf8.ValidString(k) {
			invalidKeys[k] = v
		}

		if v.Type() == pcommon.ValueTypeStr && !utf8.ValidString(v.Str()) {
			sanitized := []byte(v.Str())
			newVal := attributes.PutEmptyBytes(k)
			newVal.Append(sanitized...)
		}
		return true
	})

	i := 1
	for k, v := range invalidKeys {
		sanitized := []byte(k + ":")
		switch v.Type() {
		case pcommon.ValueTypeBytes:
			sanitized = append(sanitized, v.Bytes().AsRaw()...)
		default:
			sanitized = append(sanitized, []byte(v.AsString())...)
		}

		newKey := fmt.Sprintf("%s-%d", invalidTagKey, i)
		newVal := attributes.PutEmptyBytes(newKey)
		newVal.Append(sanitized...)
		i++
		attributes.Remove(k)
	}
}
