// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"strconv"

	"github.com/docker/go-units"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	traceSizeExceededSpanName = "trace-size-exceeded"
)

// CorrectMaxSize returns an Adjuster that validates whether a trace size
// is within the allowed maximum size limit.
//
// This adjuster calculates the serialized size of the trace and compares it
// against the configured maximum size. If the trace exceeds the limit, it
// replaces the trace with a minimal trace containing a single span that
// indicates the trace was too large to process.
//
// Parameters:
//   - maxTraceSize: The maximum allowed trace size (e.g., "10MB", "512KB").
func CorrectMaxSize(maxTraceSize string) Adjuster {
	return Func(func(traces ptrace.Traces) {
		maxTraceSizeBytes, err := units.RAMInBytes(maxTraceSize)
		if err != nil || maxTraceSizeBytes <= 0 {
			return
		}

		marshaler := &ptrace.ProtoMarshaler{}
		traceSizeBytes := int64(marshaler.TracesSize(traces))

		if traceSizeBytes <= maxTraceSizeBytes {
			return
		}

		newTraces := ptrace.NewTraces()
		rs := newTraces.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()
		span := ss.Spans().AppendEmpty()
		span.SetName(traceSizeExceededSpanName)

		traceID, found := extractTraceID(traces)
		if found {
			span.SetTraceID(traceID)
		}

		span.Attributes().PutStr(
			"warning",
			"Trace size exceeds max limit. Query underlying storage directly.",
		)

		span.Attributes().PutStr(
			"max_allowed_bytes",
			strconv.FormatInt(maxTraceSizeBytes, 10),
		)

		span.Attributes().PutStr(
			"actual_trace_size_bytes",
			strconv.FormatInt(traceSizeBytes, 10),
		)

		newTraces.MoveTo(traces)
	})
}

func extractTraceID(traces ptrace.Traces) (pcommon.TraceID, bool) {
	if traces.ResourceSpans().Len() == 0 {
		return pcommon.TraceID{}, false
	}

	rs := traces.ResourceSpans().At(0)
	if rs.ScopeSpans().Len() == 0 {
		return pcommon.TraceID{}, false
	}

	ss := rs.ScopeSpans().At(0)
	if ss.Spans().Len() == 0 {
		return pcommon.TraceID{}, false
	}

	return ss.Spans().At(0).TraceID(), true
}
