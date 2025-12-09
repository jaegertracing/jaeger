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
	"go.opentelemetry.io/collector/pdata/xpdata"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
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
	require.Equal(t, expected.Kind, jptrace.SpanKindToString(actual.Kind()))
	require.Equal(t, expected.StartTime.UnixNano(), actual.StartTimestamp().AsTime().UnixNano())
	require.Equal(t, expected.StatusCode, actual.Status().Code().String())
	require.Equal(t, expected.StatusMessage, actual.Status().Message())
	require.Equal(t, time.Duration(expected.Duration), actual.EndTimestamp().AsTime().Sub(actual.StartTimestamp().AsTime()))

	requireBoolAttrs(t, expected.Attributes.BoolKeys, expected.Attributes.BoolValues, actual.Attributes())
	requireDoubleAttrs(t, expected.Attributes.DoubleKeys, expected.Attributes.DoubleValues, actual.Attributes())
	requireIntAttrs(t, expected.Attributes.IntKeys, expected.Attributes.IntValues, actual.Attributes())
	requireStrAttrs(t, expected.Attributes.StrKeys, expected.Attributes.StrValues, actual.Attributes())
	requireComplexAttrs(t, expected.Attributes.ComplexKeys, expected.Attributes.ComplexValues, actual.Attributes())

	require.Len(t, expected.EventNames, actual.Events().Len())
	for i, e := range actual.Events().All() {
		require.Equal(t, expected.EventNames[i], e.Name())
		require.Equal(t, expected.EventTimestamps[i].UnixNano(), e.Timestamp().AsTime().UnixNano())

		requireBoolAttrs(t, expected.EventAttributes.BoolKeys[i], expected.EventAttributes.BoolValues[i], e.Attributes())
		requireDoubleAttrs(t, expected.EventAttributes.DoubleKeys[i], expected.EventAttributes.DoubleValues[i], e.Attributes())
		requireIntAttrs(t, expected.EventAttributes.IntKeys[i], expected.EventAttributes.IntValues[i], e.Attributes())
		requireStrAttrs(t, expected.EventAttributes.StrKeys[i], expected.EventAttributes.StrValues[i], e.Attributes())
		requireComplexAttrs(t, expected.EventAttributes.ComplexKeys[i], expected.EventAttributes.ComplexValues[i], e.Attributes())
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
		case strings.HasPrefix(k, "@map@"):
			key := strings.TrimPrefix(expectedKeys[i], "@map@")
			val, ok := attrs.Get(key)
			require.True(t, ok)

			m := &xpdata.JSONUnmarshaler{}
			expectedVal, err := m.UnmarshalValue([]byte(expectedVals[i]))
			require.NoError(t, err)
			require.True(t, expectedVal.Map().Equal(val.Map()))
		case strings.HasPrefix(k, "@slice@"):
			key := strings.TrimPrefix(expectedKeys[i], "@slice@")
			val, ok := attrs.Get(key)
			require.True(t, ok)

			m := &xpdata.JSONUnmarshaler{}
			expectedVal, err := m.UnmarshalValue([]byte(expectedVals[i]))
			require.NoError(t, err)
			require.True(t, expectedVal.Slice().Equal(val.Slice()))
		default:
			t.Fatalf("unsupported complex attribute key: %s", k)
		}
	}
}
