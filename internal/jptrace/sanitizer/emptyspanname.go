// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	emptySpanName = "empty-span-name"
)

// NewEmptySpanNameSanitizer returns a sanitizer function that replaces
// empty span names with a placeholder string.
func NewEmptySpanNameSanitizer() Func {
	return sanitizeEmptySpanName
}

func sanitizeEmptySpanName(traces ptrace.Traces) ptrace.Traces {
	if !tracesNeedSpanNameSanitization(traces) {
		return traces
	}

	var workingTraces ptrace.Traces

	if traces.IsReadOnly() {
		workingTraces = ptrace.NewTraces()
		traces.CopyTo(workingTraces)
	} else {
		workingTraces = traces
	}

	for _, resourceSpan := range workingTraces.ResourceSpans().All() {
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if span.Name() == "" {
					span.SetName(emptySpanName)
				}
			}
		}
	}

	return workingTraces
}

func tracesNeedSpanNameSanitization(traces ptrace.Traces) bool {
	for _, resourceSpan := range traces.ResourceSpans().All() {
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if span.Name() == "" {
					return true
				}
			}
		}
	}
	return false
}
