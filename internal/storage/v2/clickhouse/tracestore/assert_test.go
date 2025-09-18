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

func requireTracesEqual(t *testing.T, expected []spanRow, actual []ptrace.Traces) {
	t.Helper()

	require.Len(t, actual, len(expected))

	for i, e := range expected {
		resources := actual[i].ResourceSpans()
		require.Equal(t, 1, resources.Len())

		scopes := resources.At(0).ScopeSpans()
		require.Equal(t, 1, scopes.Len())
		requireScopeEqual(t, e, scopes.At(0).Scope())

		spans := scopes.At(0).Spans()
		require.Equal(t, 1, spans.Len())

		requireSpanEqual(t, e, spans.At(0))
	}
}

func requireScopeEqual(t *testing.T, expected spanRow, actual pcommon.InstrumentationScope) {
	t.Helper()

	require.Equal(t, expected.scopeName, actual.Name())
	require.Equal(t, expected.scopeVersion, actual.Version())
}

func requireSpanEqual(t *testing.T, expected spanRow, actual ptrace.Span) {
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

	requireBoolAttrs(t, expected.boolAttributeKeys, expected.boolAttributeValues, actual.Attributes())
	requireDoubleAttrs(t, expected.doubleAttributeKeys, expected.doubleAttributeValues, actual.Attributes())
	requireIntAttrs(t, expected.intAttributeKeys, expected.intAttributeValues, actual.Attributes())
	requireStrAttrs(t, expected.strAttributeKeys, expected.strAttributeValues, actual.Attributes())
	requireComplexAttrs(t, expected.complexAttributeKeys, expected.complexAttributeValues, actual.Attributes())

	require.Equal(t, actual.Events().Len(), len(expected.eventNames))
	for i, e := range actual.Events().All() {
		require.Equal(t, expected.eventNames[i], e.Name())
		require.Equal(t, expected.eventTimestamps[i].UnixNano(), e.Timestamp().AsTime().UnixNano())

		requireBoolAttrs(t, expected.eventBoolAttributeKeys[i], expected.eventBoolAttributeValues[i], e.Attributes())
		requireDoubleAttrs(t, expected.eventDoubleAttributeKeys[i], expected.eventDoubleAttributeValues[i], e.Attributes())
		requireIntAttrs(t, expected.eventIntAttributeKeys[i], expected.eventIntAttributeValues[i], e.Attributes())
		requireStrAttrs(t, expected.eventStrAttributeKeys[i], expected.eventStrAttributeValues[i], e.Attributes())
		requireComplexAttrs(t, expected.eventComplexAttributeKeys[i], expected.eventComplexAttributeValues[i], e.Attributes())
	}

	require.Equal(t, actual.Links().Len(), len(expected.linkSpanIDs))
	for i, l := range actual.Links().All() {
		require.Equal(t, expected.linkTraceIDs[i], l.TraceID().String())
		require.Equal(t, expected.linkSpanIDs[i], l.SpanID().String())
		require.Equal(t, expected.linkTraceStates[i], l.TraceState().AsRaw())
	}
}

func requireBoolAttrs(t *testing.T, expectedKeys []string, expectedVals []bool, attrs pcommon.Map) {
	for i, k := range expectedKeys {
		val, ok := attrs.Get(k)
		require.True(t, ok)
		require.Equal(t, expectedVals[i], val.Bool())
	}
}

func requireDoubleAttrs(t *testing.T, expectedKeys []string, expectedVals []float64, attrs pcommon.Map) {
	for i, k := range expectedKeys {
		val, ok := attrs.Get(k)
		require.True(t, ok)
		require.Equal(t, expectedVals[i], val.Double())
	}
}

func requireIntAttrs(t *testing.T, expectedKeys []string, expectedVals []int64, attrs pcommon.Map) {
	for i, k := range expectedKeys {
		val, ok := attrs.Get(k)
		require.True(t, ok)
		require.Equal(t, expectedVals[i], val.Int())
	}
}

func requireStrAttrs(t *testing.T, expectedKeys []string, expectedVals []string, attrs pcommon.Map) {
	for i, k := range expectedKeys {
		val, ok := attrs.Get(k)
		require.True(t, ok)
		require.Equal(t, expectedVals[i], val.Str())
	}
}

func requireComplexAttrs(t *testing.T, expectedKeys []string, expectedVals []string, attrs pcommon.Map) {
	for i, k := range expectedKeys {
		switch {
		case strings.HasPrefix(k, "@bytes@"):
			key := strings.TrimPrefix(k, "@bytes@")
			val, ok := attrs.Get(key)
			require.True(t, ok)
			encoded := base64.StdEncoding.EncodeToString(val.Bytes().AsRaw())
			require.Equal(t, expectedVals[i], encoded)
		}
	}
}
