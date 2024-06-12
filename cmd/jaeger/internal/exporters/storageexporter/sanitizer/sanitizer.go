// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// SanitizeTraces is a function that performs enrichment, clean-up, or normalization of trace data.
type SanitizeTraces func(traces ptrace.Traces) ptrace.Traces

// NewStandardSanitizers are automatically applied by SpanProcessor.
func NewStandardSanitizers() []SanitizeTraces {
	return []SanitizeTraces{
		NewEmptyServiceNameSanitizer(),
		NewUTF8Sanitizer(),
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
