// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

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

		require.Equal(t, expected.ID, row[0])                        // SpanID
		require.Equal(t, expected.TraceID, row[1])                   // TraceID
		require.Equal(t, expected.TraceState, row[2])                // TraceState
		require.Equal(t, expected.ParentSpanID, row[3])              // ParentSpanID
		require.Equal(t, expected.Name, row[4])                      // Name
		require.Equal(t, strings.ToLower(expected.Kind), row[5])     // Kind
		require.Equal(t, expected.StartTime, row[6])                 // StartTimestamp
		require.Equal(t, expected.StatusCode, row[7])                // Status code
		require.Equal(t, expected.StatusMessage, row[8])             // Status message
		require.EqualValues(t, expected.Duration, row[9])            // Duration
		require.Equal(t, expected.Attributes.BoolKeys, row[10])      // Bool attribute keys
		require.Equal(t, expected.Attributes.BoolValues, row[11])    // Bool attribute values
		require.Equal(t, expected.Attributes.DoubleKeys, row[12])    // Double attribute keys
		require.Equal(t, expected.Attributes.DoubleValues, row[13])  // Double attribute values
		require.Equal(t, expected.Attributes.IntKeys, row[14])       // Int attribute keys
		require.Equal(t, expected.Attributes.IntValues, row[15])     // Int attribute values
		require.Equal(t, expected.Attributes.StrKeys, row[16])       // Str attribute keys
		require.Equal(t, expected.Attributes.StrValues, row[17])     // Str attribute values
		require.Equal(t, expected.Attributes.ComplexKeys, row[18])   // Complex attribute keys
		require.Equal(t, expected.Attributes.ComplexValues, row[19]) // Complex attribute values
		require.Equal(t, expected.EventNames, row[20])               // Event names
		require.Equal(t, expected.EventTimestamps, row[21])          // Event timestamps
		require.Equal(t,
			toTuple(expected.EventAttributes.BoolKeys, expected.EventAttributes.BoolValues),
			row[22],
		) // Event bool attributes
		require.Equal(t,
			toTuple(expected.EventAttributes.DoubleKeys, expected.EventAttributes.DoubleValues),
			row[23],
		) // Event double attributes
		require.Equal(t,
			toTuple(expected.EventAttributes.IntKeys, expected.EventAttributes.IntValues),
			row[24],
		) // Event int attributes
		require.Equal(t,
			toTuple(expected.EventAttributes.StrKeys, expected.EventAttributes.StrValues),
			row[25],
		) // Event str attributes
		require.Equal(t,
			toTuple(expected.EventAttributes.ComplexKeys, expected.EventAttributes.ComplexValues),
			row[26],
		) // Event complex attributes
		require.Equal(t, expected.LinkTraceIDs, row[27])    // Link TraceIDs
		require.Equal(t, expected.LinkSpanIDs, row[28])     // Link SpanIDs
		require.Equal(t, expected.LinkTraceStates, row[29]) // Link TraceStates
		require.Equal(t,
			toTuple(expected.LinkAttributes.BoolKeys, expected.LinkAttributes.BoolValues),
			row[30],
		) // Link bool attributes
		require.Equal(t,
			toTuple(expected.LinkAttributes.DoubleKeys, expected.LinkAttributes.DoubleValues),
			row[31],
		) // Link double attributes
		require.Equal(t,
			toTuple(expected.LinkAttributes.IntKeys, expected.LinkAttributes.IntValues),
			row[32],
		) // Link int attributes
		require.Equal(t,
			toTuple(expected.LinkAttributes.StrKeys, expected.LinkAttributes.StrValues),
			row[33],
		) // Link str attributes
		require.Equal(t,
			toTuple(expected.LinkAttributes.ComplexKeys, expected.LinkAttributes.ComplexValues),
			row[34],
		) // Link complex attributes
		require.Equal(t, expected.ServiceName, row[35])                      // Service name
		require.Equal(t, expected.ResourceAttributes.BoolKeys, row[36])      // Resource bool attribute keys
		require.Equal(t, expected.ResourceAttributes.BoolValues, row[37])    // Resource bool attribute values
		require.Equal(t, expected.ResourceAttributes.DoubleKeys, row[38])    // Resource double attribute keys
		require.Equal(t, expected.ResourceAttributes.DoubleValues, row[39])  // Resource double attribute values
		require.Equal(t, expected.ResourceAttributes.IntKeys, row[40])       // Resource int attribute keys
		require.Equal(t, expected.ResourceAttributes.IntValues, row[41])     // Resource int attribute values
		require.Equal(t, expected.ResourceAttributes.StrKeys, row[42])       // Resource str attribute keys
		require.Equal(t, expected.ResourceAttributes.StrValues, row[43])     // Resource str attribute values
		require.Equal(t, expected.ResourceAttributes.ComplexKeys, row[44])   // Resource complex attribute keys
		require.Equal(t, expected.ResourceAttributes.ComplexValues, row[45]) // Resource complex attribute values
		require.Equal(t, expected.ScopeName, row[46])                        // Scope name
		require.Equal(t, expected.ScopeVersion, row[47])                     // Scope version
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
