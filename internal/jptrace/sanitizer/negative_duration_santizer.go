// Copyright (c) 2025 The Jaeger Authors.

package sanitizer

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

const (
	minDuration = time.Duration(1)
)

// NewNegativeDurationSanitizer returns a sanitizer function that performs the
// following sanitizationson the trace data:
//   - Checks all spans in the ResourceSpans to ensure that the start timestamp
//     is less than the end timestamp. If not, it sets the end timestamp to
//     start timestamp plus a minimum duration of 1 nanosecond.
func NewNegativeDurationSanitizer() Func {
	return sanitizeNegativeDuration
}

func sanitizeNegativeDuration(traces ptrace.Traces) ptrace.Traces {
	if !tracesNeedDurationSanitization(traces) {
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
				sanitizeDuration(&span)
			}
		}
	}

	return workingTraces
}

func tracesNeedDurationSanitization(traces ptrace.Traces) bool {
	for _, resourceSpan := range traces.ResourceSpans().All() {
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				if spanNeedsDurationSanitization(span) {
					return true
				}
			}
		}
	}
	return false
}

func spanNeedsDurationSanitization(span ptrace.Span) bool {
	start := span.StartTimestamp().AsTime()
	end := span.EndTimestamp().AsTime()
	return !start.Before(end)
}

func sanitizeDuration(span *ptrace.Span) {
	start := span.StartTimestamp().AsTime()
	end := span.EndTimestamp().AsTime()
	if start.Before(end) {
		return
	}

	newEnd := start.Add(minDuration)
	jptrace.AddWarnings(
		*span,
		fmt.Sprintf(
			"Negative duration detected, sanitizing end timestamp. Original end timestamp: %s, adjusted to: %s",
			end.String(), newEnd.String(),
		),
	)

	span.SetEndTimestamp(pcommon.NewTimestampFromTime(newEnd))
}
