// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"github.com/jaegertracing/jaeger/model"
)

// AddHashTag creates a sanitizer to add hash field to spans
func AddHashTag() SanitizeSpan {
	return hashingSanitizer
}

func hashingSanitizer(span *model.Span) *model.Span {
	// Check if hash already exists
	if _, found := span.GetHashTag(); found {
		return span
	}

	_, err := span.SetHashTag()
	if err != nil {
		return span
	}

	return span
}
