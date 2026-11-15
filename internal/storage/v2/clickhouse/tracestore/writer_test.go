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

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
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

func TestWriter_Success_NestedLayout(t *testing.T) {
	b := &clickhousetest.Batch{}
	conn := &clickhousetest.Driver{
		BatchResponses: map[string]*clickhousetest.BatchResponse{
			"INSERT INTO spans": {
				Batch: b,
			},
		},
	}
	w := NewWriter(conn, NestedLayout{})

	td := tracesFromSpanRows(multipleSpans)

	err := w.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	require.Len(t, conn.RecordedQueries, 1)
	clickhousetest.VerifyQuerySnapshot(t, conn.RecordedQueries[0])
	require.True(t, b.SendCalled)
	require.Len(t, b.Appended, len(multipleSpans))

	for i, expected := range multipleSpans {
		row := valuesByColumn(t, sql.InsertSpanNestedLayout, b.Appended[i])
		requireSpanColumns(t, row, expected)
		requireNestedGroups(t, row, "", expected.Attributes)
		requireNestedGroups(t, row, "resource_", expected.ResourceAttributes)
		requireNestedGroups(t, row, "scope_", expected.ScopeAttributes)
		requireNestedGroups2D(t, row, "events.", expected.EventAttributes)
		requireNestedGroups2D(t, row, "links.", expected.LinkAttributes)
	}
}

func TestWriter_Success_MapLayout(t *testing.T) {
	b := &clickhousetest.Batch{}
	conn := &clickhousetest.Driver{
		BatchResponses: map[string]*clickhousetest.BatchResponse{
			"INSERT INTO spans": {
				Batch: b,
			},
		},
	}
	w := NewWriter(conn, MapLayout{})

	require.NoError(t, w.WriteTraces(context.Background(), tracesFromSpanRows(multipleSpans)))

	clickhousetest.VerifyQuerySnapshot(t, conn.RecordedQueries[0])
	require.True(t, b.SendCalled)
	require.Len(t, b.Appended, len(multipleSpans))
	for i, expected := range multipleSpans {
		row := valuesByColumn(t, sql.InsertSpanMapLayout, b.Appended[i])
		requireSpanColumns(t, row, expected)
		requireMapGroups(t, row, "", expected.Attributes)
		requireMapGroups(t, row, "resource_", expected.ResourceAttributes)
		requireMapGroups(t, row, "scope_", expected.ScopeAttributes)
		requireMapGroups2D(t, row, "events.", expected.EventAttributes)
		requireMapGroups2D(t, row, "links.", expected.LinkAttributes)
	}
}

// valuesByColumn pairs the values of one appended row with the columns that the insert
// statement names, so that a test asserts a value by its column and not by its position.
func valuesByColumn(t *testing.T, insertStatement string, values []any) map[string]any {
	t.Helper()
	_, afterTable, found := strings.Cut(insertStatement, "spans (")
	require.True(t, found)
	columnList, _, found := strings.Cut(afterTable, ")")
	require.True(t, found)
	columns := strings.Split(columnList, ",")
	require.Len(t, values, len(columns))
	byColumn := make(map[string]any, len(columns))
	for i, column := range columns {
		byColumn[strings.TrimSpace(column)] = values[i]
	}
	return byColumn
}

// requireSpanColumns asserts the columns that hold no attributes, which every layout stores
// the same way.
func requireSpanColumns(t *testing.T, row map[string]any, expected *dbmodel.SpanRow) {
	t.Helper()
	require.Equal(t, expected.ID, row["id"])
	require.Equal(t, expected.TraceID, row["trace_id"])
	require.Equal(t, expected.TraceState, row["trace_state"])
	require.Equal(t, expected.ParentSpanID, row["parent_span_id"])
	require.Equal(t, expected.Name, row["name"])
	require.Equal(t, strings.ToLower(expected.Kind), row["kind"])
	require.Equal(t, expected.StartTime, row["start_time"])
	require.Equal(t, expected.StatusCode, row["status_code"])
	require.Equal(t, expected.StatusMessage, row["status_message"])
	require.EqualValues(t, expected.Duration, row["duration"])
	require.Equal(t, expected.EventNames, row["events.name"])
	require.Equal(t, expected.EventTimestamps, row["events.timestamp"])
	require.Equal(t, expected.LinkTraceIDs, row["links.trace_id"])
	require.Equal(t, expected.LinkSpanIDs, row["links.span_id"])
	require.Equal(t, expected.LinkTraceStates, row["links.trace_state"])
	require.Equal(t, expected.ServiceName, row["service_name"])
	require.Equal(t, expected.ScopeName, row["scope_name"])
	require.Equal(t, expected.ScopeVersion, row["scope_version"])
}

// requireNestedGroups asserts that every typed group under the prefix is a key column and a
// value column.
func requireNestedGroups(t *testing.T, row map[string]any, prefix string, attributes dbmodel.Attributes) {
	t.Helper()
	require.Equal(t, attributes.BoolKeys, row[prefix+"bool_attributes.key"])
	require.Equal(t, attributes.BoolValues, row[prefix+"bool_attributes.value"])
	require.Equal(t, attributes.DoubleKeys, row[prefix+"double_attributes.key"])
	require.Equal(t, attributes.DoubleValues, row[prefix+"double_attributes.value"])
	require.Equal(t, attributes.IntKeys, row[prefix+"int_attributes.key"])
	require.Equal(t, attributes.IntValues, row[prefix+"int_attributes.value"])
	require.Equal(t, attributes.StrKeys, row[prefix+"str_attributes.key"])
	require.Equal(t, attributes.StrValues, row[prefix+"str_attributes.value"])
	require.Equal(t, attributes.ComplexKeys, row[prefix+"complex_attributes.key"])
	require.Equal(t, attributes.ComplexValues, row[prefix+"complex_attributes.value"])
}

// requireNestedGroups2D asserts that every typed group under the prefix holds the key and
// value pairs of each event or link.
func requireNestedGroups2D(t *testing.T, row map[string]any, prefix string, attributes dbmodel.Attributes2D) {
	t.Helper()
	require.Equal(t, toTuple(attributes.BoolKeys, attributes.BoolValues), row[prefix+"bool_attributes"])
	require.Equal(t, toTuple(attributes.DoubleKeys, attributes.DoubleValues), row[prefix+"double_attributes"])
	require.Equal(t, toTuple(attributes.IntKeys, attributes.IntValues), row[prefix+"int_attributes"])
	require.Equal(t, toTuple(attributes.StrKeys, attributes.StrValues), row[prefix+"str_attributes"])
	require.Equal(t, toTuple(attributes.ComplexKeys, attributes.ComplexValues), row[prefix+"complex_attributes"])
}

// requireMapGroups asserts that every typed group under the prefix is one map value.
func requireMapGroups(t *testing.T, row map[string]any, prefix string, attributes dbmodel.Attributes) {
	t.Helper()
	require.Equal(t, orderedMap[bool]{keys: attributes.BoolKeys, values: attributes.BoolValues}, row[prefix+"bool_attributes"])
	require.Equal(t, orderedMap[float64]{keys: attributes.DoubleKeys, values: attributes.DoubleValues}, row[prefix+"double_attributes"])
	require.Equal(t, orderedMap[int64]{keys: attributes.IntKeys, values: attributes.IntValues}, row[prefix+"int_attributes"])
	require.Equal(t, orderedMap[string]{keys: attributes.StrKeys, values: attributes.StrValues}, row[prefix+"str_attributes"])
	require.Equal(t, orderedMap[string]{keys: attributes.ComplexKeys, values: attributes.ComplexValues}, row[prefix+"complex_attributes"])
}

// requireMapGroups2D asserts that every typed group under the prefix is one map per event or link.
func requireMapGroups2D(t *testing.T, row map[string]any, prefix string, attributes dbmodel.Attributes2D) {
	t.Helper()
	require.Equal(t, orderedMaps(attributes.BoolKeys, attributes.BoolValues), row[prefix+"bool_attributes"])
	require.Equal(t, orderedMaps(attributes.DoubleKeys, attributes.DoubleValues), row[prefix+"double_attributes"])
	require.Equal(t, orderedMaps(attributes.IntKeys, attributes.IntValues), row[prefix+"int_attributes"])
	require.Equal(t, orderedMaps(attributes.StrKeys, attributes.StrValues), row[prefix+"str_attributes"])
	require.Equal(t, orderedMaps(attributes.ComplexKeys, attributes.ComplexValues), row[prefix+"complex_attributes"])
}

func TestWriter_PrepareBatchError(t *testing.T) {
	conn := &clickhousetest.Driver{
		BatchResponses: map[string]*clickhousetest.BatchResponse{
			"INSERT INTO spans": {
				Batch: nil,
				Err:   assert.AnError,
			},
		},
	}
	w := NewWriter(conn, NestedLayout{})
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(multipleSpans))
	require.ErrorContains(t, err, "failed to prepare batch")
	require.ErrorIs(t, err, assert.AnError)
}

func TestWriter_AppendBatchError(t *testing.T) {
	b := &clickhousetest.Batch{AppendErr: assert.AnError}
	conn := &clickhousetest.Driver{
		BatchResponses: map[string]*clickhousetest.BatchResponse{
			"INSERT INTO spans": {
				Batch: b,
			},
		},
	}
	w := NewWriter(conn, NestedLayout{})
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(multipleSpans))
	require.ErrorContains(t, err, "failed to append span to batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, b.SendCalled)
}

func TestWriter_SendError(t *testing.T) {
	b := &clickhousetest.Batch{SendErr: assert.AnError}
	conn := &clickhousetest.Driver{
		BatchResponses: map[string]*clickhousetest.BatchResponse{
			"INSERT INTO spans": {
				Batch: b,
			},
		},
	}
	w := NewWriter(conn, NestedLayout{})
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(multipleSpans))
	require.ErrorContains(t, err, "failed to send batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, b.SendCalled)
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
