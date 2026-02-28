// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"strings"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// PruneTraceForLLM distills a large model.Trace down to a compact markdown string
// containing ONLY spans that have an error tag, plus their immediate parent spans.
// This is critical to ensure LLMs (especially local SLMs) do not exceed context limits,
// and to increase attention span on the actual problem.
func PruneTraceForLLM(trace *model.Trace) string {
	if trace == nil || len(trace.Spans) == 0 {
		return "No trace data provided."
	}

	// 1. Map all spans by ID for quick parent lookup
	spanMap := make(map[model.SpanID]*model.Span)
	errorSpanIDs := make(map[model.SpanID]struct{})

	for _, span := range trace.Spans {
		spanMap[span.SpanID] = span
		if isErrorSpan(span) {
			errorSpanIDs[span.SpanID] = struct{}{}
		}
	}

	if len(errorSpanIDs) == 0 {
		return "Trace completed successfully with no errors detected."
	}

	// 2. Collect error spans and their immediate parents
	relevantSpans := make(map[model.SpanID]*model.Span)
	for id := range errorSpanIDs {
		errSpan := spanMap[id]
		relevantSpans[id] = errSpan

		// Add immediate parent if it exists
		if errSpan.ParentSpanID() != 0 {
			parent, exists := spanMap[errSpan.ParentSpanID()]
			if exists {
				relevantSpans[parent.SpanID] = parent
			}
		}
	}

	// 3. Format as a compact human-readable string
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Trace ID: %s\n", trace.Spans[0].TraceID.String()))
	sb.WriteString("Critical Error Path Spans:\n")

	for _, span := range relevantSpans {
		sb.WriteString(fmt.Sprintf("\n- Span: %s (ID: %s) [Duration: %v]\n", span.OperationName, span.SpanID.String(), span.Duration))

		isErr := isErrorSpan(span)
		sb.WriteString(fmt.Sprintf("  HasError: %v\n", isErr))

		if len(span.Tags) > 0 {
			sb.WriteString("  Relevant Tags:\n")
			for _, tag := range span.Tags {
				// Only include error-related or highly contextual tags to save tokens
				if tag.Key == "error" || tag.Key == "http.status_code" || tag.Key == "http.url" || tag.Key == "db.statement" || strings.Contains(strings.ToLower(tag.Key), "exception") {
					sb.WriteString(fmt.Sprintf("    %s: %s\n", tag.Key, tag.AsString()))
				}
			}
		}

		if len(span.Logs) > 0 {
			sb.WriteString("  Logs:\n")
			for _, log := range span.Logs {
				for _, field := range log.Fields {
					sb.WriteString(fmt.Sprintf("    [%s] %s: %s\n", log.Timestamp.Format("15:04:05.000"), field.Key, field.AsString()))
				}
			}
		}
	}

	return sb.String()
}

func isErrorSpan(span *model.Span) bool {
	for _, tag := range span.Tags {
		if tag.Key == "error" && tag.AsString() == "true" {
			return true
		}
	}
	return false
}
