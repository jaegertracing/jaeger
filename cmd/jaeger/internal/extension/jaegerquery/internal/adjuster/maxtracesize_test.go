// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxTraceSizeAdjuster(t *testing.T) {
	tests := []struct {
		name          string
		maxTraceSize  string
		spanCount     int
		expectWarning bool
	}{
		{
			name:          "trace within limit",
			maxTraceSize:  "10MB",
			spanCount:     1,
			expectWarning: false,
		},
		{
			name:          "trace exceeds limit",
			maxTraceSize:  "1B",
			spanCount:     10,
			expectWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spanProtos := make([]testSpan, tt.spanCount)
			for i := 0; i < tt.spanCount; i++ {
				spanProtos[i] = testSpan{
					id:        [8]byte{byte(i + 1)},
					startTime: i,
					duration:  10,
				}
			}

			traces := makeTrace(spanProtos)

			originalTraceID := traces.ResourceSpans().
				At(0).
				ScopeSpans().
				At(0).
				Spans().
				At(0).
				TraceID()

			adjuster := CorrectMaxSize(tt.maxTraceSize)
			adjuster.Adjust(traces)

			spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

			if tt.expectWarning {
				assert.Equal(t, 1, spans.Len(), "should reduce trace to single warning span")

				wSpan := spans.At(0)

				assert.NotEqual(t, originalTraceID, wSpan.TraceID())
				assert.Equal(t, traceSizeExceededSpanName, wSpan.Name())

				warningAttr, ok := wSpan.Attributes().Get("warning")
				assert.True(t, ok)
				assert.NotEmpty(t, warningAttr.Str())

				maxBytesAttr, ok := wSpan.Attributes().Get("max_allowed_bytes")
				assert.True(t, ok)
				assert.NotEmpty(t, maxBytesAttr.Str())

				actualSizeAttr, ok := wSpan.Attributes().Get("actual_trace_size_bytes")
				assert.True(t, ok)
				assert.NotEmpty(t, actualSizeAttr.Str())
			} else {
				assert.GreaterOrEqual(t, spans.Len(), 1)
				assert.NotEqual(t, traceSizeExceededSpanName, spans.At(0).Name())
			}
		})
	}
}
