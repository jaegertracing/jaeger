package sanitizer

import (
	"unicode/utf8"

	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	invalidOperation = "InvalidOperationName"
	invalidTagKey    = "InvalidTagKey"
)

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
					newVal := span.Attributes().PutEmptyBytes(invalidOperation)
					newVal.Append(sanitized...)
					span.SetName(invalidOperation)
				}

				serviceName, ok := span.Attributes().Get(string(otelsemconv.ServiceNameKey))
				if ok &&
					serviceName.Type() == pcommon.ValueTypeStr &&
					!utf8.ValidString(serviceName.Str()) {
					sanitized := []byte(serviceName.Str())
					newVal := span.Attributes().PutEmptyBytes(string(otelsemconv.ServiceNameKey))
					newVal.Append(sanitized...)
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

	for k, v := range invalidKeys {
		sanitized := []byte(k + ":")
		switch v.Type() {
		case pcommon.ValueTypeBytes:
			sanitized = append(sanitized, v.Bytes().AsRaw()...)
		default:
			sanitized = append(sanitized, []byte(v.Str())...)
		}

		attributes.Remove(k)
		newVal := attributes.PutEmptyBytes(invalidTagKey)
		newVal.Append(sanitized...)
	}
}
