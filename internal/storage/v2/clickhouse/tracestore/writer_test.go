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

		require.Equal(t, expected.ID, row[0])                    // SpanID
		require.Equal(t, expected.TraceID, row[1])               // TraceID
		require.Equal(t, expected.TraceState, row[2])            // TraceState
		require.Equal(t, expected.ParentSpanID, row[3])          // ParentSpanID
		require.Equal(t, expected.Name, row[4])                  // Name
		require.Equal(t, strings.ToLower(expected.Kind), row[5]) // Kind
		require.Equal(t, expected.StartTime, row[6])             // StartTimestamp
		require.Equal(t, expected.StatusCode, row[7])            // Status code
		require.Equal(t, expected.StatusMessage, row[8])         // Status message
		require.EqualValues(t, expected.Duration, row[9])        // Duration
		require.Equal(t, expected.Attributes.Keys, row[10])      // Attribute keys
		require.Equal(t, expected.Attributes.Values, row[11])    // Attribute values
		require.Equal(t, expected.Attributes.Types, row[12])     // Attribute types
		require.Equal(t, expected.EventNames, row[13])           // Event names
		require.Equal(t, expected.EventTimestamps, row[14])      // Event timestamps
		require.Equal(t,
			toTuple(expected.EventAttributes.Keys, expected.EventAttributes.Values, expected.EventAttributes.Types),
			row[15],
		) // Event attributes
		require.Equal(t, expected.LinkTraceIDs, row[16])    // Link TraceIDs
		require.Equal(t, expected.LinkSpanIDs, row[17])     // Link SpanIDs
		require.Equal(t, expected.LinkTraceStates, row[18]) // Link TraceStates
		require.Equal(t,
			toTuple(expected.LinkAttributes.Keys, expected.LinkAttributes.Values, expected.LinkAttributes.Types),
			row[19],
		) // Link attributes
		require.Equal(t, expected.ServiceName, row[20])               // Service name
		require.Equal(t, expected.ResourceAttributes.Keys, row[21])   // Resource attribute keys
		require.Equal(t, expected.ResourceAttributes.Values, row[22]) // Resource attribute values
		require.Equal(t, expected.ResourceAttributes.Types, row[23])  // Resource attribute types
		require.Equal(t, expected.ScopeName, row[24])                 // Scope name
		require.Equal(t, expected.ScopeVersion, row[25])              // Scope version
		require.Equal(t, expected.ScopeAttributes.Keys, row[26])      // Scope attribute keys
		require.Equal(t, expected.ScopeAttributes.Values, row[27])    // Scope attribute values
		require.Equal(t, expected.ScopeAttributes.Types, row[28])     // Scope attribute types
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
		types    [][]string
		expected [][][]any
	}{
		{
			name:     "empty slices",
			keys:     [][]string{},
			values:   [][]int{},
			types:    [][]string{},
			expected: [][][]any{},
		},
		{
			name:     "single empty inner slice",
			keys:     [][]string{{}},
			values:   [][]int{{}},
			types:    [][]string{{}},
			expected: [][][]any{{}},
		},
		{
			name:   "single element",
			keys:   [][]string{{"key1"}},
			values: [][]int{{42}},
			types:  [][]string{{"int"}},
			expected: [][][]any{
				{
					{"key1", 42, "int"},
				},
			},
		},
		{
			name:   "multiple elements in single slice",
			keys:   [][]string{{"key1", "key2", "key3"}},
			values: [][]int{{10, 20, 30}},
			types:  [][]string{{"int", "int", "int"}},
			expected: [][][]any{
				{
					{"key1", 10, "int"},
					{"key2", 20, "int"},
					{"key3", 30, "int"},
				},
			},
		},
		{
			name:   "multiple slices with multiple elements",
			keys:   [][]string{{"key1", "key2"}, {"key3"}, {"key4", "key5", "key6"}},
			values: [][]int{{1, 2}, {3}, {4, 5, 6}},
			types:  [][]string{{"int", "int"}, {"int"}, {"int", "int", "int"}},
			expected: [][][]any{
				{
					{"key1", 1, "int"},
					{"key2", 2, "int"},
				},
				{
					{"key3", 3, "int"},
				},
				{
					{"key4", 4, "int"},
					{"key5", 5, "int"},
					{"key6", 6, "int"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toTuple(tt.keys, tt.values, tt.types)
			require.Equal(t, tt.expected, result)
		})
	}
}
