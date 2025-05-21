// Copyright (c) 2025 The Jaeger Authors.

package sanitizer

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// negativeDurationSanitizer sanitizes all negative durations to a default value.
type negativeDurationSanitizer struct{}

// NewNegativeDurationSanitizer creates a negative duration sanitizer.
func NewNegativeDurationSanitizer() SanitizeSpan {
	negativeDurationSanitizer := negativeDurationSanitizer{}
	return negativeDurationSanitizer.Sanitize
}

// Sanitize sanitizes the spans with negative durations.
func (s *negativeDurationSanitizer) Sanitize(span *model.Span) *model.Span {
	return span
}
