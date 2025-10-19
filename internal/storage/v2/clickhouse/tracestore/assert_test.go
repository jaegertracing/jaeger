// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func requireTracesEqual(t *testing.T, expected []*dbmodel.SpanRow, actual []ptrace.Traces) {
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

func requireScopeEqual(t *testing.T, expected *dbmodel.SpanRow, actual pcommon.InstrumentationScope) {
	t.Helper()

	require.Equal(t, expected.ScopeName, actual.Name())
	require.Equal(t, expected.ScopeVersion, actual.Version())
}

func requireSpanEqual(t *testing.T, expected *dbmodel.SpanRow, actual ptrace.Span) {
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

	requireBoolAttrs(t, expected.BoolAttributeKeys, expected.BoolAttributeValues, actual.Attributes())
	requireDoubleAttrs(t, expected.DoubleAttributeKeys, expected.DoubleAttributeValues, actual.Attributes())
	requireIntAttrs(t, expected.IntAttributeKeys, expected.IntAttributeValues, actual.Attributes())
	requireStrAttrs(t, expected.StrAttributeKeys, expected.StrAttributeValues, actual.Attributes())
	requireComplexAttrs(t, expected.ComplexAttributeKeys, expected.ComplexAttributeValues, actual.Attributes())

	require.Len(t, expected.EventNames, actual.Events().Len())
	for i, e := range actual.Events().All() {
		require.Equal(t, expected.EventNames[i], e.Name())
		require.Equal(t, expected.EventTimestamps[i].UnixNano(), e.Timestamp().AsTime().UnixNano())

		requireBoolAttrs(t, expected.EventBoolAttributeKeys[i], expected.EventBoolAttributeValues[i], e.Attributes())
		requireDoubleAttrs(t, expected.EventDoubleAttributeKeys[i], expected.EventDoubleAttributeValues[i], e.Attributes())
		requireIntAttrs(t, expected.EventIntAttributeKeys[i], expected.EventIntAttributeValues[i], e.Attributes())
		requireStrAttrs(t, expected.EventStrAttributeKeys[i], expected.EventStrAttributeValues[i], e.Attributes())
		requireComplexAttrs(t, expected.EventComplexAttributeKeys[i], expected.EventComplexAttributeValues[i], e.Attributes())
	}

	require.Len(t, expected.LinkSpanIDs, actual.Links().Len())
	for i, l := range actual.Links().All() {
		require.Equal(t, expected.LinkTraceIDs[i], l.TraceID().String())
		require.Equal(t, expected.LinkSpanIDs[i], l.SpanID().String())
		require.Equal(t, expected.LinkTraceStates[i], l.TraceState().AsRaw())
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
		require.InEpsilon(t, expectedVals[i], val.Double(), 1e-9)
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
			key := strings.TrimPrefix(expectedKeys[i], "@bytes@")
			val, ok := attrs.Get(key)
			require.True(t, ok)
			decoded, err := base64.StdEncoding.DecodeString(expectedVals[i])
			require.NoError(t, err)
			require.Equal(t, decoded, val.Bytes().AsRaw())
		default:
			t.Fatalf("unsupported complex attribute key: %s", k)
		}
	}
}
