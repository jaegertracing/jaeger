// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

type StorageIntegrationV2 struct {
	TraceReader tracestore.Reader
}

// extractSpansFromTraces returns a slice of spans contained in one otel trace
func extractSpansFromTraces(traces ptrace.Traces) []ptrace.Span {
	var allSpans []ptrace.Span

	// Iterate through ResourceSpans
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)

		// Iterate through ScopeSpans within ResourceSpans
		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			scopeSpan := scopeSpans.At(j)

			// Iterate through Spans within ScopeSpans
			spans := scopeSpan.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				allSpans = append(allSpans, span)
			}
		}
	}

	return allSpans
}
