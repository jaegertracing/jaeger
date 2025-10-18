// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/base64"
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

		for i := 0; i < len(r.boolAttributeKeys); i++ {
			span.Attributes().PutBool(r.boolAttributeKeys[i], r.boolAttributeValues[i])
		}
		for i := 0; i < len(r.doubleAttributeKeys); i++ {
			span.Attributes().PutDouble(r.doubleAttributeKeys[i], r.doubleAttributeValues[i])
		}
		for i := 0; i < len(r.intAttributeKeys); i++ {
			span.Attributes().PutInt(r.intAttributeKeys[i], r.intAttributeValues[i])
		}
		for i := 0; i < len(r.strAttributeKeys); i++ {
			span.Attributes().PutStr(r.strAttributeKeys[i], r.strAttributeValues[i])
		}
		for i := 0; i < len(r.complexAttributeKeys); i++ {
			if strings.HasPrefix(r.complexAttributeKeys[i], "@bytes@") {
				decoded, err := base64.StdEncoding.DecodeString(r.complexAttributeValues[i])
				require.NoError(t, err)
				k := strings.TrimPrefix(r.complexAttributeKeys[i], "@bytes@")
				span.Attributes().PutEmptyBytes(k).FromRaw(decoded)
			}
		}

		for i, e := range r.eventNames {
			event := span.Events().AppendEmpty()
			event.SetName(e)
			event.SetTimestamp(pcommon.NewTimestampFromTime(r.eventTimestamps[i]))
			for j := 0; j < len(r.eventBoolAttributeKeys[i]); j++ {
				event.Attributes().PutBool(r.eventBoolAttributeKeys[i][j], r.eventBoolAttributeValues[i][j])
			}
			for j := 0; j < len(r.eventDoubleAttributeKeys[i]); j++ {
				event.Attributes().PutDouble(r.eventDoubleAttributeKeys[i][j], r.eventDoubleAttributeValues[i][j])
			}
			for j := 0; j < len(r.eventIntAttributeKeys[i]); j++ {
				event.Attributes().PutInt(r.eventIntAttributeKeys[i][j], r.eventIntAttributeValues[i][j])
			}
			for j := 0; j < len(r.eventStrAttributeKeys[i]); j++ {
				event.Attributes().PutStr(r.eventStrAttributeKeys[i][j], r.eventStrAttributeValues[i][j])
			}
			for j := 0; j < len(r.eventComplexAttributeKeys[i]); j++ {
				if strings.HasPrefix(r.eventComplexAttributeKeys[i][j], "@bytes@") {
					decoded, err := base64.StdEncoding.DecodeString(r.eventComplexAttributeValues[i][j])
					require.NoError(t, err)
					k := strings.TrimPrefix(r.eventComplexAttributeKeys[i][j], "@bytes@")
					event.Attributes().PutEmptyBytes(k).FromRaw(decoded)
				}
			}
		}

		for i, l := range r.linkTraceIDs {
			link := span.Links().AppendEmpty()
			traceID, err := hex.DecodeString(l)
			require.NoError(t, err)
			link.SetTraceID(pcommon.TraceID(traceID))
			spanID, err := hex.DecodeString(r.linkSpanIDs[i])
			require.NoError(t, err)
			link.SetSpanID(pcommon.SpanID(spanID))
			link.TraceState().FromRaw(r.linkTraceStates[i])
		}
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

		require.Equal(t, expected.id, row[0])                      // SpanID
		require.Equal(t, expected.traceID, row[1])                 // TraceID
		require.Equal(t, expected.traceState, row[2])              // TraceState
		require.Equal(t, expected.parentSpanID, row[3])            // ParentSpanID
		require.Equal(t, expected.name, row[4])                    // Name
		require.Equal(t, strings.ToLower(expected.kind), row[5])   // Kind
		require.Equal(t, expected.startTime, row[6])               // StartTimestamp
		require.Equal(t, expected.statusCode, row[7])              // Status code
		require.Equal(t, expected.statusMessage, row[8])           // Status message
		require.EqualValues(t, expected.rawDuration, row[9])       // Duration
		require.Equal(t, expected.serviceName, row[10])            // Service name
		require.Equal(t, expected.scopeName, row[11])              // Scope name
		require.Equal(t, expected.scopeVersion, row[12])           // Scope version
		require.Equal(t, expected.boolAttributeKeys, row[13])      // Bool attribute keys
		require.Equal(t, expected.boolAttributeValues, row[14])    // Bool attribute values
		require.Equal(t, expected.doubleAttributeKeys, row[15])    // Double attribute keys
		require.Equal(t, expected.doubleAttributeValues, row[16])  // Double attribute values
		require.Equal(t, expected.intAttributeKeys, row[17])       // Int attribute keys
		require.Equal(t, expected.intAttributeValues, row[18])     // Int attribute values
		require.Equal(t, expected.strAttributeKeys, row[19])       // Str attribute keys
		require.Equal(t, expected.strAttributeValues, row[20])     // Str attribute values
		require.Equal(t, expected.complexAttributeKeys, row[21])   // Complex attribute keys
		require.Equal(t, expected.complexAttributeValues, row[22]) // Complex attribute values
		require.Equal(t, expected.eventNames, row[23])             // Event names
		require.Equal(t, expected.eventTimestamps, row[24])        // Event timestamps
		require.Equal(t,
			toTuple(expected.eventBoolAttributeKeys, expected.eventBoolAttributeValues),
			row[25],
		) // Bool attributes
		require.Equal(t,
			toTuple(expected.eventDoubleAttributeKeys, expected.eventDoubleAttributeValues),
			row[26],
		) // Double attributes
		require.Equal(t,
			toTuple(expected.eventIntAttributeKeys, expected.eventIntAttributeValues),
			row[27],
		) // Int attributes
		require.Equal(t,
			toTuple(expected.eventStrAttributeKeys, expected.eventStrAttributeValues),
			row[28],
		) // Str attributes
		require.Equal(t,
			toTuple(expected.eventComplexAttributeKeys, expected.eventComplexAttributeValues),
			row[29],
		) // Complex attribute
		require.Equal(t, expected.linkTraceIDs, row[30])    // Link TraceIDs
		require.Equal(t, expected.linkSpanIDs, row[31])     // Link SpanIDs
		require.Equal(t, expected.linkTraceStates, row[32]) // Link TraceStates
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

func TestToTuple(t *testing.T) {
	tests := []struct {
		name     string
		keys     [][]string
		values   [][]int
		expected [][][]any
	}{
		{
			name:     "empty slices",
			keys:     [][]string{},
			values:   [][]int{},
			expected: [][][]any{},
		},
		{
			name:     "single empty inner slice",
			keys:     [][]string{{}},
			values:   [][]int{{}},
			expected: [][][]any{{}},
		},
		{
			name:   "single element",
			keys:   [][]string{{"key1"}},
			values: [][]int{{42}},
			expected: [][][]any{
				{
					{"key1", 42},
				},
			},
		},
		{
			name:   "multiple elements in single slice",
			keys:   [][]string{{"key1", "key2", "key3"}},
			values: [][]int{{10, 20, 30}},
			expected: [][][]any{
				{
					{"key1", 10},
					{"key2", 20},
					{"key3", 30},
				},
			},
		},
		{
			name:   "multiple slices with multiple elements",
			keys:   [][]string{{"key1", "key2"}, {"key3"}, {"key4", "key5", "key6"}},
			values: [][]int{{1, 2}, {3}, {4, 5, 6}},
			expected: [][][]any{
				{
					{"key1", 1},
					{"key2", 2},
				},
				{
					{"key3", 3},
				},
				{
					{"key4", 4},
					{"key5", 5},
					{"key6", 6},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toTuple(tt.keys, tt.values)
			require.Equal(t, tt.expected, result)
		})
	}
}
