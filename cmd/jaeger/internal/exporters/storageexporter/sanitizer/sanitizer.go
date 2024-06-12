// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// SanitizeSpan sanitizes/normalizes spans. Any business logic that needs to be applied to normalize the contents of a
// span should implement this interface.
type SanitizeTraces func(traces ptrace.Traces) ptrace.Traces

// NewStandardSanitizers are automatically applied by SpanProcessor.
func NewStandardSanitizers(logger *zap.Logger, cache Cache) []SanitizeTraces {
	return []SanitizeTraces{
		NewEmptyServiceNameSanitizer(),
		NewUTF8Sanitizer(logger),
		serviceNameSanitizer{cache: cache}.Sanitize,
	}
}

// NewChainedSanitizer creates a Sanitizer from the variadic list of passed Sanitizers.
// If the list only has one element, it is returned directly to minimize indirection.
func NewChainedSanitizer(sanitizers ...SanitizeTraces) SanitizeTraces {
	if len(sanitizers) == 1 {
		return sanitizers[0]
	}
	return func(traces ptrace.Traces) ptrace.Traces {
		for _, s := range sanitizers {
			traces = s(traces)
		}
		return traces
	}
}
