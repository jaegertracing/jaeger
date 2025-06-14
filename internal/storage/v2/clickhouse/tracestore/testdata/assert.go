// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

import (
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
	require.Equal(t, expected.StatusCode, actual.Status().Code().String(), "status code mismatch")
	require.Equal(t, expected.StatusMessage, actual.Status().Message(), "status message mismatch")
	require.Equal(t, time.Duration(expected.RawDuration), actual.EndTimestamp().AsTime().Sub(actual.StartTimestamp().AsTime()))

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
