// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// SanitizeSpan sanitizes/normalizes spans. Any business logic that needs to be applied to normalize the contents of a
// span should implement this interface.
type SanitizeSpan func(span *model.Span) *model.Span

// NewStandardSanitizers are automatically applied by SpanProcessor.
func NewStandardSanitizers() []SanitizeSpan {
	return []SanitizeSpan{
		NewEmptyServiceNameSanitizer(),
		// AddHashTag(),
	}
}

// NewChainedSanitizer creates a Sanitizer from the variadic list of passed Sanitizers.
// If the list only has one element, it is returned directly to minimize indirection.
func NewChainedSanitizer(sanitizers ...SanitizeSpan) SanitizeSpan {
	if len(sanitizers) == 1 {
		return sanitizers[0]
	}
	return func(span *model.Span) *model.Span {
		for _, s := range sanitizers {
			span = s(span)
		}
		return span
	}
}
