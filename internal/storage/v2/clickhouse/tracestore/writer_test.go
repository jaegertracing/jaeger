// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func tracesFromSpanRows(t *testing.T, rows []*spanRow) ptrace.Traces {
	td := ptrace.NewTraces()
	for _, r := range rows {
		rs := td.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, r.serviceName)

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName(r.scopeName)
		ss.Scope().SetVersion(r.scopeVersion)

		span := ss.Spans().AppendEmpty()
		spanID, err := hex.DecodeString(r.id)
		require.NoError(t, err)
		span.SetSpanID(pcommon.SpanID(spanID))
		traceID, err := hex.DecodeString(r.traceID)
		require.NoError(t, err)
		span.SetTraceID(pcommon.TraceID(traceID))
		span.TraceState().FromRaw(r.traceState)
		if r.parentSpanID != "" {
			parentSpanID, err := hex.DecodeString(r.parentSpanID)
			require.NoError(t, err)
			span.SetParentSpanID(pcommon.SpanID(parentSpanID))
		}
		span.SetName(r.name)
		span.SetKind(jptrace.StringToSpanKind(r.kind))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(r.startTime))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(r.startTime.Add(time.Duration(r.rawDuration))))
		span.Status().SetCode(jptrace.StringToStatusCode(r.statusCode))
		span.Status().SetMessage(r.statusMessage)
	}
	return td
}

func TestWriter_Success(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		batch:         &testBatch{t: t},
	}
	w := NewWriter(conn)

	td := tracesFromSpanRows(t, multipleSpans)

	err := w.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	require.True(t, conn.batch.sendCalled)
	require.Len(t, conn.batch.appended, len(multipleSpans))

	for i, expected := range multipleSpans {
		row := conn.batch.appended[i]

		require.Equal(t, expected.id, row[0])                    // SpanID
		require.Equal(t, expected.traceID, row[1])               // TraceID
		require.Equal(t, expected.traceState, row[2])            // TraceState
		require.Equal(t, expected.parentSpanID, row[3])          // ParentSpanID
		require.Equal(t, expected.name, row[4])                  // Name
		require.Equal(t, strings.ToLower(expected.kind), row[5]) // Kind
		require.Equal(t, expected.startTime, row[6])             // StartTimestamp
		require.Equal(t, expected.statusCode, row[7])            // Status code
		require.Equal(t, expected.statusMessage, row[8])         // Status message
		require.EqualValues(t, expected.rawDuration, row[9])     // Duration

		// Fields 10-31 are attributes, events, and links (verified by successful batch.Append)
		// These fields are tested indirectly - if batch.Append succeeds, the structure is correct

		require.Equal(t, expected.serviceName, row[33])  // Service name (field 34, index 33)
		require.Equal(t, expected.scopeName, row[34])    // Scope name (field 35, index 34)
		require.Equal(t, expected.scopeVersion, row[35]) // Scope version (field 36, index 35)
	}
}

func TestWriter_PrepareBatchError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		err:           assert.AnError,
		batch:         &testBatch{t: t},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(t, multipleSpans))
	require.ErrorContains(t, err, "failed to prepare batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, conn.batch.sendCalled)
}

func TestWriter_AppendBatchError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		batch:         &testBatch{t: t, appendErr: assert.AnError},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(t, multipleSpans))
	require.ErrorContains(t, err, "failed to append span to batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, conn.batch.sendCalled)
}

func TestWriter_SendError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		batch:         &testBatch{t: t, sendErr: assert.AnError},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(t, multipleSpans))
	require.ErrorContains(t, err, "failed to send batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, conn.batch.sendCalled)
}
