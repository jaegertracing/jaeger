// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Func is a function that performs enrichment, clean-up, or normalization of trace data.
type Func func(traces ptrace.Traces) ptrace.Traces

// Sanitize is a function that applies all sanitizers to the given trace data.
var Sanitize = NewChainedSanitizer(NewStandardSanitizers()...)

// NewStandardSanitizers returns a list of all the sanitizers that are used by the
// storage exporter.
func NewStandardSanitizers() []Func {
	return []Func{
		NewEmptyServiceNameSanitizer(),
		NewUTF8Sanitizer(),
		NewNegativeDurationSanitizer(),
	}
}

// NewChainedSanitizer creates a Sanitizer from the variadic list of passed Sanitizers.
// If the list only has one element, it is returned directly to minimize indirection.
func NewChainedSanitizer(sanitizers ...Func) Func {
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
