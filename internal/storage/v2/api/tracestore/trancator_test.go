// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type mockWriter struct {
	written ptrace.Traces
}

func (m *mockWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	m.written = td
	return nil
}

func TestTruncatingWriter(t *testing.T) {
	tests := []struct {
		name     string
		maxBytes int
		input    string
		expected string
	}{
		{
			name:     "unlimited mode (0) keeps original payload",
			maxBytes: 0,
			input:    "this is a massive genai prompt payload",
			expected: "this is a massive genai prompt payload",
		},
		{
			name:     "under limit keeps original payload",
			maxBytes: 100,
			input:    "short string",
			expected: "short string",
		},
		{
			name:     "over limit successfully truncates and appends suffix",
			maxBytes: 4,
			input:    "longstring", // length = 10
			expected: "long [truncated: 10 bytes]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Generate an OpenTelemetry Trace Payload with nested attributes
			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("resource.key", tt.input)

			ss := rs.ScopeSpans().AppendEmpty()
			ss.Scope().Attributes().PutStr("scope.key", tt.input)

			span := ss.Spans().AppendEmpty()
			span.Attributes().PutStr("span.key", tt.input)

			event := span.Events().AppendEmpty()
			event.Attributes().PutStr("event.key", tt.input)

			link := span.Links().AppendEmpty()
			link.Attributes().PutStr("link.key", tt.input)

			// 2. Wrap a mock database writer with our middleware
			mw := &mockWriter{}
			writer := NewTruncatingWriter(mw, tt.maxBytes)

			// 3. Execute the write
			err := writer.WriteTraces(context.Background(), td)
			require.NoError(t, err)

			// 4. Assert all levels were intercepted and truncated properly
			val, ok := mw.written.ResourceSpans().At(0).Resource().Attributes().Get("resource.key")
			require.True(t, ok)
			assert.Equal(t, tt.expected, val.Str(), "Resource attributes failed")

			val, ok = mw.written.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes().Get("scope.key")
			require.True(t, ok)
			assert.Equal(t, tt.expected, val.Str(), "Scope attributes failed")

			val, ok = mw.written.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes().Get("span.key")
			require.True(t, ok)
			assert.Equal(t, tt.expected, val.Str(), "Span attributes failed")

			val, ok = mw.written.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Events().At(0).Attributes().Get("event.key")
			require.True(t, ok)
			assert.Equal(t, tt.expected, val.Str(), "Event attributes failed")

			val, ok = mw.written.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Links().At(0).Attributes().Get("link.key")
			require.True(t, ok)
			assert.Equal(t, tt.expected, val.Str(), "Link attributes failed")
		})
	}
}
