// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestOTELAttributeAdjuster(t *testing.T) {
	tests := []struct {
		name     string
		input    ptrace.Traces
		expected ptrace.Traces
	}{
		{
			name: "span with otel library attributes",
			input: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				span.Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
				span.Attributes().PutStr(string(otelsemconv.TelemetrySDKNameKey), "opentelemetry")
				span.Attributes().PutStr(string(otelsemconv.TelemetrySDKVersionKey), "1.27.0")
				span.Attributes().PutStr(string(otelsemconv.TelemetryDistroNameKey), "opentelemetry")
				span.Attributes().PutStr(string(otelsemconv.TelemetryDistroVersionKey), "blah")
				span.Attributes().PutStr("another_key", "another_value")
				return traces
			}(),
			expected: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				span.Attributes().PutStr("another_key", "another_value")
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKNameKey), "opentelemetry")
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKVersionKey), "1.27.0")
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetryDistroNameKey), "opentelemetry")
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetryDistroVersionKey), "blah")
				return traces
			}(),
		},
		{
			name: "span without otel library attributes",
			input: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				return traces
			}(),
			expected: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				return traces
			}(),
		},
		{
			name: "span with conflicting otel library attributes",
			input: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				span.Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Java")
				return traces
			}(),
			expected: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				span.Attributes().PutStr("jaeger.adjuster.warning", "conflicting attribute values for telemetry.sdk.language")
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
				return traces
			}(),
		},
		{
			name: "span with non-conflicting otel library attributes",
			input: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				span.Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
				return traces
			}(),
			expected: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.Attributes().PutStr("random_key", "random_value")
				rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
				return traces
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adjuster := OTELAttribute()
			input := ptrace.NewTraces()
			test.input.CopyTo(input)
			result, err := adjuster.Adjust(input)
			require.NoError(t, err)
			assert.EqualValues(t, test.expected, result)
		})
	}
}
