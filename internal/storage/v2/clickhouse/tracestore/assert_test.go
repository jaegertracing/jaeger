// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func RequireScopeEqual(t *testing.T, expected SpanRow, actual pcommon.InstrumentationScope) {
	t.Helper()

	require.Equal(t, expected.ScopeName, actual.Name())
	require.Equal(t, expected.ScopeVersion, actual.Version())
}

func RequireSpanEqual(t *testing.T, expected SpanRow, actual ptrace.Span) {
	t.Helper()

	require.Equal(t, expected.ID, actual.SpanID().String())
	require.Equal(t, expected.TraceID, actual.TraceID().String())
	require.Equal(t, expected.TraceState, actual.TraceState().AsRaw())
	require.Equal(t, expected.ParentSpanID, actual.ParentSpanID().String())
	require.Equal(t, expected.Name, actual.Name())
	require.Equal(t, expected.Kind, actual.Kind().String())
	require.Equal(t, expected.StartTime.UnixNano(), actual.StartTimestamp().AsTime().UnixNano())
	require.Equal(t, expected.StatusCode, actual.Status().Code().String())
	require.Equal(t, expected.StatusMessage, actual.Status().Message())
	require.Equal(t, time.Duration(expected.RawDuration), actual.EndTimestamp().AsTime().Sub(actual.StartTimestamp().AsTime()))

	for i, k := range expected.BoolAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.BoolAttributeValues[i], val.Bool())
	}

	for i, k := range expected.DoubleAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.DoubleAttributeValues[i], val.Double())
	}

	for i, k := range expected.IntAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.IntAttributeValues[i], val.Int())
	}

	for i, k := range expected.StrAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.StrAttributeValues[i], val.Str())
	}

	for i, k := range expected.ComplexAttributeKeys {
		switch {
		case strings.HasPrefix(k, "@bytes@"):
			key := strings.TrimPrefix(k, "@bytes@")
			val, ok := actual.Attributes().Get(key)
			require.True(t, ok)
			encoded := base64.StdEncoding.EncodeToString(val.Bytes().AsRaw())
			require.Equal(t, expected.ComplexAttributeValues[i], encoded)
		}
	}

	require.Equal(t, actual.Events().Len(), len(expected.EventNames))
	for i, e := range actual.Events().All() {
		require.Equal(t, expected.EventNames[i], e.Name())
		require.Equal(t, expected.EventTimestamps[i].UnixNano(), e.Timestamp().AsTime().UnixNano())
	}

	require.Equal(t, actual.Links().Len(), len(expected.LinkSpanIDs))
	for i, l := range actual.Links().All() {
		require.Equal(t, expected.LinkTraceIDs[i], l.TraceID().String())
		require.Equal(t, expected.LinkSpanIDs[i], l.SpanID().String())
		require.Equal(t, expected.LinkTraceStates[i], l.TraceState().AsRaw())
	}
}

func RequireTracesEqual(t *testing.T, expected []SpanRow, actual []ptrace.Traces) {
	t.Helper()

	require.Len(t, actual, len(expected))

	for i, e := range expected {
		resources := actual[i].ResourceSpans()
		require.Equal(t, 1, resources.Len())

		scopes := resources.At(0).ScopeSpans()
		require.Equal(t, 1, scopes.Len())
		RequireScopeEqual(t, e, scopes.At(0).Scope())

		spans := scopes.At(0).Spans()
		require.Equal(t, 1, spans.Len())

		RequireSpanEqual(t, e, spans.At(0))
	}
}
