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

func RequireScopeEqual(t *testing.T, expected spanRow, actual pcommon.InstrumentationScope) {
	t.Helper()

	require.Equal(t, expected.scopeName, actual.Name())
	require.Equal(t, expected.scopeVersion, actual.Version())
}

func RequireSpanEqual(t *testing.T, expected spanRow, actual ptrace.Span) {
	t.Helper()

	require.Equal(t, expected.id, actual.SpanID().String())
	require.Equal(t, expected.traceID, actual.TraceID().String())
	require.Equal(t, expected.traceState, actual.TraceState().AsRaw())
	require.Equal(t, expected.parentSpanID, actual.ParentSpanID().String())
	require.Equal(t, expected.name, actual.Name())
	require.Equal(t, expected.kind, actual.Kind().String())
	require.Equal(t, expected.startTime.UnixNano(), actual.StartTimestamp().AsTime().UnixNano())
	require.Equal(t, expected.statusCode, actual.Status().Code().String())
	require.Equal(t, expected.statusMessage, actual.Status().Message())
	require.Equal(t, time.Duration(expected.rawDuration), actual.EndTimestamp().AsTime().Sub(actual.StartTimestamp().AsTime()))

	for i, k := range expected.boolAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.boolAttributeValues[i], val.Bool())
	}

	for i, k := range expected.doubleAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.doubleAttributeValues[i], val.Double())
	}

	for i, k := range expected.intAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.intAttributeValues[i], val.Int())
	}

	for i, k := range expected.strAttributeKeys {
		val, ok := actual.Attributes().Get(k)
		require.True(t, ok)
		require.Equal(t, expected.strAttributeValues[i], val.Str())
	}

	for i, k := range expected.complexAttributeKeys {
		switch {
		case strings.HasPrefix(k, "@bytes@"):
			key := strings.TrimPrefix(k, "@bytes@")
			val, ok := actual.Attributes().Get(key)
			require.True(t, ok)
			encoded := base64.StdEncoding.EncodeToString(val.Bytes().AsRaw())
			require.Equal(t, expected.complexAttributeValues[i], encoded)
		}
	}

	require.Equal(t, actual.Events().Len(), len(expected.eventNames))
	for i, e := range actual.Events().All() {
		require.Equal(t, expected.eventNames[i], e.Name())
		require.Equal(t, expected.eventTimestamps[i].UnixNano(), e.Timestamp().AsTime().UnixNano())
	}

	require.Equal(t, actual.Links().Len(), len(expected.linkSpanIDs))
	for i, l := range actual.Links().All() {
		require.Equal(t, expected.linkTraceIDs[i], l.TraceID().String())
		require.Equal(t, expected.linkSpanIDs[i], l.SpanID().String())
		require.Equal(t, expected.linkTraceStates[i], l.TraceState().AsRaw())
	}
}

func RequireTracesEqual(t *testing.T, expected []spanRow, actual []ptrace.Traces) {
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
