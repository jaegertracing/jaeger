// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

func putAttributes(
	t *testing.T,
	attrs pcommon.Map,
	boolKeys []string, boolValues []bool,
	doubleKeys []string, doubleValues []float64,
	intKeys []string, intValues []int64,
	strKeys []string, strValues []string,
	complexKeys []string, complexValues []string,
) {
	t.Helper()
	for i := 0; i < len(boolKeys); i++ {
		attrs.PutBool(boolKeys[i], boolValues[i])
	}
	for i := 0; i < len(doubleKeys); i++ {
		attrs.PutDouble(doubleKeys[i], doubleValues[i])
	}
	for i := 0; i < len(intKeys); i++ {
		attrs.PutInt(intKeys[i], intValues[i])
	}
	for i := 0; i < len(strKeys); i++ {
		attrs.PutStr(strKeys[i], strValues[i])
	}
	for i := 0; i < len(complexKeys); i++ {
		if strings.HasPrefix(complexKeys[i], "@bytes@") {
			decoded, err := base64.StdEncoding.DecodeString(complexValues[i])
			require.NoError(t, err)
			k := strings.TrimPrefix(complexKeys[i], "@bytes@")
			attrs.PutEmptyBytes(k).FromRaw(decoded)
		}
	}
}

func tracesFromSpanRows(rows []*dbmodel.SpanRow) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans()
	for _, r := range rows {
		trace := dbmodel.FromRow(r)
		srcRS := trace.ResourceSpans()
		for i := 0; i < srcRS.Len(); i++ {
			srcRS.At(i).CopyTo(rs.AppendEmpty())
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

	td := tracesFromSpanRows(multipleSpans)

	err := w.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	require.True(t, conn.batch.sendCalled)
	require.Len(t, conn.batch.appended, len(multipleSpans))

	for i, expected := range multipleSpans {
		row := conn.batch.appended[i]

		require.Equal(t, expected.ID, row[0])                      // SpanID
		require.Equal(t, expected.TraceID, row[1])                 // TraceID
		require.Equal(t, expected.TraceState, row[2])              // TraceState
		require.Equal(t, expected.ParentSpanID, row[3])            // ParentSpanID
		require.Equal(t, expected.Name, row[4])                    // Name
		require.Equal(t, strings.ToLower(expected.Kind), row[5])   // Kind
		require.Equal(t, expected.StartTime, row[6])               // StartTimestamp
		require.Equal(t, expected.StatusCode, row[7])              // Status code
		require.Equal(t, expected.StatusMessage, row[8])           // Status message
		require.EqualValues(t, expected.RawDuration, row[9])       // Duration
		require.Equal(t, expected.ServiceName, row[10])            // Service name
		require.Equal(t, expected.ScopeName, row[11])              // Scope name
		require.Equal(t, expected.ScopeVersion, row[12])           // Scope version
		require.Equal(t, expected.BoolAttributeKeys, row[13])      // Bool attribute keys
		require.Equal(t, expected.BoolAttributeValues, row[14])    // Bool attribute values
		require.Equal(t, expected.DoubleAttributeKeys, row[15])    // Double attribute keys
		require.Equal(t, expected.DoubleAttributeValues, row[16])  // Double attribute values
		require.Equal(t, expected.IntAttributeKeys, row[17])       // Int attribute keys
		require.Equal(t, expected.IntAttributeValues, row[18])     // Int attribute values
		require.Equal(t, expected.StrAttributeKeys, row[19])       // Str attribute keys
		require.Equal(t, expected.StrAttributeValues, row[20])     // Str attribute values
		require.Equal(t, expected.ComplexAttributeKeys, row[21])   // Complex attribute keys
		require.Equal(t, expected.ComplexAttributeValues, row[22]) // Complex attribute values
		require.Equal(t, expected.EventNames, row[23])             // Event names
		require.Equal(t, expected.EventTimestamps, row[24])        // Event timestamps
		require.Equal(t,
			toTuple(expected.EventBoolAttributeKeys, expected.EventBoolAttributeValues),
			row[25],
		) // Event bool attributes
		require.Equal(t,
			toTuple(expected.EventDoubleAttributeKeys, expected.EventDoubleAttributeValues),
			row[26],
		) // Event double attributes
		require.Equal(t,
			toTuple(expected.EventIntAttributeKeys, expected.EventIntAttributeValues),
			row[27],
		) // Event int attributes
		require.Equal(t,
			toTuple(expected.EventStrAttributeKeys, expected.EventStrAttributeValues),
			row[28],
		) // Event str attributes
		require.Equal(t,
			toTuple(expected.EventComplexAttributeKeys, expected.EventComplexAttributeValues),
			row[29],
		) // Event complex attributes
		require.Equal(t, expected.LinkTraceIDs, row[30])    // Link TraceIDs
		require.Equal(t, expected.LinkSpanIDs, row[31])     // Link SpanIDs
		require.Equal(t, expected.LinkTraceStates, row[32]) // Link TraceStates
		require.Equal(t,
			toTuple(expected.LinkBoolAttributeKeys, expected.LinkBoolAttributeValues),
			row[33],
		) // Link bool attributes
		require.Equal(t,
			toTuple(expected.LinkDoubleAttributeKeys, expected.LinkDoubleAttributeValues),
			row[34],
		) // Link double attributes
		require.Equal(t,
			toTuple(expected.LinkIntAttributeKeys, expected.LinkIntAttributeValues),
			row[35],
		) // Link int attributes
		require.Equal(t,
			toTuple(expected.LinkStrAttributeKeys, expected.LinkStrAttributeValues),
			row[36],
		) // Link str attributes
		require.Equal(t,
			toTuple(expected.LinkComplexAttributeKeys, expected.LinkComplexAttributeValues),
			row[37],
		) // Link complex attributes
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
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(multipleSpans))
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
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(multipleSpans))
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
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(multipleSpans))
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
