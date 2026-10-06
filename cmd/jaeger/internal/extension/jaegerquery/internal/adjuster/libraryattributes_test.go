// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
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

	adjuster := MoveLibraryAttributes()
	adjuster.Adjust(traces)

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

	adjuster := MoveLibraryAttributes()
	adjuster.Adjust(traces)

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

	adjuster := MoveLibraryAttributes()
	adjuster.Adjust(traces)

	resultSpan := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	resultSpanAttributes := resultSpan.Attributes()
	require.Equal(t, 3, resultSpanAttributes.Len())
	val, ok := resultSpanAttributes.Get("random_key")
	require.True(t, ok)
	require.Equal(t, "random_value", val.Str())

	// value remains in the span
	val, ok = resultSpanAttributes.Get(string(otelsemconv.TelemetrySDKLanguageKey))
	require.True(t, ok)
	require.Equal(t, "Java", val.Str())

	warnings := jptrace.GetWarnings(resultSpan)
	require.True(t, ok)
	require.Len(t, warnings, 1)
	require.Equal(t, "conflicting values between Span and Resource for attribute telemetry.sdk.language", warnings[0])

	resultResourceAttributes := traces.ResourceSpans().At(0).Resource().Attributes()
	val, ok = resultResourceAttributes.Get(string(otelsemconv.TelemetrySDKLanguageKey))
	require.True(t, ok)
	require.Equal(t, "Go", val.Str())
}

func TestResourceAttributesAdjuster_SpanAndResourceWithNonStringLibraryAttributes(t *testing.T) {
	key := string(otelsemconv.TelemetrySDKNameKey)
	putMap := func(s string) func(pcommon.Map) {
		return func(attrs pcommon.Map) { attrs.PutEmptyMap(key).PutStr("name", s) }
	}
	putSlice := func(s string) func(pcommon.Map) {
		return func(attrs pcommon.Map) { attrs.PutEmptySlice(key).AppendEmpty().SetStr(s) }
	}
	putBytes := func(s string) func(pcommon.Map) {
		return func(attrs pcommon.Map) { attrs.PutEmptyBytes(key).FromRaw([]byte(s)) }
	}
	putInt := func(i int64) func(pcommon.Map) {
		return func(attrs pcommon.Map) { attrs.PutInt(key, i) }
	}
	tests := []struct {
		name     string
		resource func(pcommon.Map)
		span     func(pcommon.Map)
	}{
		{name: "equal maps", resource: putMap("opentelemetry"), span: putMap("opentelemetry")},
		{name: "different maps", resource: putMap("opentelemetry"), span: putMap("other")},
		{name: "equal slices", resource: putSlice("opentelemetry"), span: putSlice("opentelemetry")},
		{name: "different slices", resource: putSlice("opentelemetry"), span: putSlice("other")},
		{name: "equal bytes", resource: putBytes("opentelemetry"), span: putBytes("opentelemetry")},
		{name: "different bytes", resource: putBytes("opentelemetry"), span: putBytes("other")},
		{name: "equal ints", resource: putInt(1), span: putInt(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			rs := traces.ResourceSpans().AppendEmpty()
			tt.resource(rs.Resource().Attributes())
			span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			tt.span(span.Attributes())

			wantResource := pcommon.NewMap()
			tt.resource(wantResource)
			wantSpan := pcommon.NewMap()
			tt.span(wantSpan)

			require.NotPanics(t, func() { MoveLibraryAttributes().Adjust(traces) })

			require.Equal(t,
				[]string{"conflicting values between Span and Resource for attribute " + key},
				jptrace.GetWarnings(span))

			want, _ := wantSpan.Get(key)
			got, ok := span.Attributes().Get(key)
			require.True(t, ok)
			require.Equal(t, want.AsRaw(), got.AsRaw())

			want, _ = wantResource.Get(key)
			got, ok = rs.Resource().Attributes().Get(key)
			require.True(t, ok)
			require.Equal(t, want.AsRaw(), got.AsRaw())
		})
	}
}

func TestResourceAttributesAdjuster_SpanWithNonConflictingLibraryAttributes(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr("random_key", "random_value")
	span.Attributes().PutStr(string(otelsemconv.TelemetrySDKLanguageKey), "Go")

	adjuster := MoveLibraryAttributes()
	adjuster.Adjust(traces)

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
