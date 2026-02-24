// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestEmptySpanNameSanitizer(t *testing.T) {
	tests := []struct {
		name             string
		spanName         string
		expectedSpanName string
	}{
		{
			name:             "empty span name",
			spanName:         "",
			expectedSpanName: "empty-span-name",
		},
		{
			name:             "non-empty span name",
			spanName:         "my-operation",
			expectedSpanName: "my-operation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			span := traces.
				ResourceSpans().
				AppendEmpty().
				ScopeSpans().
				AppendEmpty().
				Spans().
				AppendEmpty()
			span.SetName(test.spanName)

			sanitizer := NewEmptySpanNameSanitizer()
			sanitized := sanitizer(traces)

			actualSpan := sanitized.
				ResourceSpans().
				At(0).
				ScopeSpans().
				At(0).
				Spans().
				At(0)
			require.Equal(t, test.expectedSpanName, actualSpan.Name())
		})
	}
}

func TestEmptySpanNameSanitizer_ReadOnly(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.
		ResourceSpans().
		AppendEmpty().
		ScopeSpans().
		AppendEmpty().
		Spans().
		AppendEmpty()
	span.SetName("")

	traces.MarkReadOnly()

	sanitizer := NewEmptySpanNameSanitizer()
	result := sanitizer(traces)

	// The original read-only traces should still have an empty span name.
	assert.Empty(t, traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name())
	// The returned traces should have the placeholder span name.
	assert.Equal(t, "empty-span-name", result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name())
}

func TestEmptySpanNameSanitizer_NoModificationNeeded(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.
		ResourceSpans().
		AppendEmpty().
		ScopeSpans().
		AppendEmpty().
		Spans().
		AppendEmpty()
	span.SetName("valid-span")

	traces.MarkReadOnly()

	sanitizer := NewEmptySpanNameSanitizer()
	result := sanitizer(traces)

	assert.Equal(t, "valid-span", result.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name())
	assert.True(t, result.IsReadOnly())
}
