// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jotlp"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

func TestResourceAttributesAdjuster_SpanWithLibraryAttributes(t *testing.T) {
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

	adjuster := ResourceAttributes()
	err := adjuster.Adjust(traces)
	require.NoError(t, err)

	resultSpanAttributes := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	require.Equal(t, 2, resultSpanAttributes.Len())
	val, ok := resultSpanAttributes.Get("random_key")
	require.True(t, ok)
	require.Equal(t, "random_value", val.Str())

	val, ok = resultSpanAttributes.Get("another_key")
	require.True(t, ok)
	require.Equal(t, "another_value", val.Str())

	resultResourceAttributes := traces.ResourceSpans().At(0).Resource().Attributes()

	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetrySDKLanguageKey))
	require.True(t, ok)
	require.Equal(t, "Go", val.Str())

	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetrySDKNameKey))
	require.True(t, ok)
	require.Equal(t, "opentelemetry", val.Str())

	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetrySDKVersionKey))
	require.True(t, ok)
	require.Equal(t, "1.27.0", val.Str())

	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetryDistroNameKey))
	require.True(t, ok)
	require.Equal(t, "opentelemetry", val.Str())

	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetryDistroVersionKey))
	require.True(t, ok)
	require.Equal(t, "blah", val.Str())
}

func TestResourceAttributesAdjuster_SpanWithoutLibraryAttributes(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr("random_key", "random_value")

	adjuster := ResourceAttributes()
	err := adjuster.Adjust(traces)
	require.NoError(t, err)

	resultSpanAttributes := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	require.Equal(t, 1, resultSpanAttributes.Len())
	val, ok := resultSpanAttributes.Get("random_key")
	require.True(t, ok)
	require.Equal(t, "random_value", val.Str())
}

func TestResourceAttributesAdjuster_SpanWithConflictingLibraryAttributes(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr("random_key", "random_value")
	span.Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Java")

	adjuster := ResourceAttributes()
	err := adjuster.Adjust(traces)
	require.NoError(t, err)

	resultSpanAttributes := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	require.Equal(t, 3, resultSpanAttributes.Len())
	val, ok := resultSpanAttributes.Get("random_key")
	require.True(t, ok)
	require.Equal(t, "random_value", val.Str())

	// value remains in the span
	val, ok = resultSpanAttributes.Get(string(otelsemconv.TelemetrySDKLanguageKey))
	require.True(t, ok)
	require.Equal(t, "Java", val.Str())

	val, ok = resultSpanAttributes.Get(jotlp.WarningsAttribute)
	require.True(t, ok)
	warnings := val.Slice()
	require.Equal(t, 1, warnings.Len())
	require.Equal(t, "conflicting values between Span and Resource for attribute telemetry.sdk.language", warnings.At(0).Str())

	resultResourceAttributes := traces.ResourceSpans().At(0).Resource().Attributes()
	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetrySDKLanguageKey))
	require.True(t, ok)
	require.Equal(t, "Go", val.Str())
}

func TestResourceAttributesAdjuster_SpanWithNonConflictingLibraryAttributes(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr("random_key", "random_value")
	span.Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")

	adjuster := ResourceAttributes()
	err := adjuster.Adjust(traces)
	require.NoError(t, err)

	resultSpanAttributes := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	require.Equal(t, 1, resultSpanAttributes.Len())
	val, ok := resultSpanAttributes.Get("random_key")
	require.True(t, ok)
	require.Equal(t, "random_value", val.Str())

	resultResourceAttributes := traces.ResourceSpans().At(0).Resource().Attributes()
	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetrySDKLanguageKey))
	require.True(t, ok)
	require.Equal(t, "Go", val.Str())
}
