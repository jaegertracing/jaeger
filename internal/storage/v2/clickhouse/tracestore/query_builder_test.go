// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

func TestBuildFindTraceIDsQuery_MarshalErrors(t *testing.T) {
	orig := marshalValueForQuery
	t.Cleanup(func() { marshalValueForQuery = orig })
	marshalValueForQuery = func(pcommon.Value) (string, error) {
		return "", assert.AnError
	}

	t.Run("marshal slice error", func(t *testing.T) {
		attrs := pcommon.NewMap()
		s := attrs.PutEmptySlice("bad_slice")
		s.AppendEmpty()

		reader := NewReader(&clickhousetest.Driver{}, testReaderConfig)
		_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: attrs})

		require.Error(t, err)
		require.ErrorContains(t, err, "failed to marshal slice attribute")
	})

	t.Run("marshal map error", func(t *testing.T) {
		attrs := pcommon.NewMap()
		m := attrs.PutEmptyMap("bad_map")
		m.PutEmpty("key")

		reader := NewReader(&clickhousetest.Driver{}, testReaderConfig)
		_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: attrs})

		require.Error(t, err)
		require.ErrorContains(t, err, "failed to marshal map attribute")
	})
}

func TestBuildFindTraceIDsQuery_AttributeMetadataError(t *testing.T) {
	td := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: nil,
				Err:  assert.AnError,
			},
		},
	}

	reader := NewReader(td, testReaderConfig)
	_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: buildTestAttributes()})
	require.ErrorContains(t, err, "failed to get attribute metadata")
}

func TestBuildStringAttributeCondition_Fallbacks(t *testing.T) {
	cases := []struct {
		name      string
		attrValue string
		metadata  attributeMetadata
	}{
		{
			name:      "parse bool fails falls back to str",
			attrValue: "not-bool",
			metadata: attributeMetadata{
				"k": {span: []pcommon.ValueType{pcommon.ValueTypeBool}},
			},
		},
		{
			name:      "parse double fails falls back to str",
			attrValue: "not-float",
			metadata: attributeMetadata{
				"k": {span: []pcommon.ValueType{pcommon.ValueTypeDouble}},
			},
		},
		{
			name:      "parse int fails falls back to str",
			attrValue: "not-int",
			metadata: attributeMetadata{
				"k": {span: []pcommon.ValueType{pcommon.ValueTypeInt}},
			},
		},
		{
			name:      "unsupported type falls back to str",
			attrValue: "whatever",
			metadata: attributeMetadata{
				"k": {span: []pcommon.ValueType{pcommon.ValueTypeEmpty}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attr := pcommon.NewValueStr(tc.attrValue)
			var q sqlBuilder
			var args []any

			args = q.buildStringAttributeCondition(args, "k", attr, tc.metadata)

			query := q.String()
			assert.Contains(t, query, "str_attributes")
			assert.Contains(t, query, "resource_str_attributes")
			assert.Contains(t, query, "scope_str_attributes")
			assert.Contains(t, query, "events")
			assert.Contains(t, query, "links")
			assert.Equal(t, []any{
				"k", tc.attrValue,
				"k", tc.attrValue,
				"k", tc.attrValue,
				"k", tc.attrValue,
				"k", tc.attrValue,
			}, args)
		})
	}
}

func TestAttributeColumnType(t *testing.T) {
	for valueType, want := range map[pcommon.ValueType]string{
		pcommon.ValueTypeBool:   "bool",
		pcommon.ValueTypeDouble: "double",
		pcommon.ValueTypeInt:    "int",
		pcommon.ValueTypeStr:    "str",
		pcommon.ValueTypeBytes:  "complex",
		pcommon.ValueTypeMap:    "complex",
		pcommon.ValueTypeSlice:  "complex",
		pcommon.ValueTypeEmpty:  "",
	} {
		assert.Equal(t, want, attributeColumnType(valueType), valueType.String())
	}
}

func TestBuildGetTracesQuery(t *testing.T) {
	tests := []struct {
		name         string
		params       tracestore.GetTraceParams
		expectedSQL  string
		expectedArgs []any
	}{
		{
			name: "without time range",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
			},
			expectedSQL:  sql.SelectSpansByTraceID,
			expectedArgs: []any{traceID},
		},
		{
			name: "with both start and end",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   now.Add(-1 * time.Hour),
				End:     now,
			},
			expectedSQL:  sql.SelectSpansByTraceID + "\n\tAND s.start_time >= fromUnixTimestamp64Nano(?)\n\tAND s.start_time <= fromUnixTimestamp64Nano(?)",
			expectedArgs: []any{traceID, now.Add(-1 * time.Hour).UnixNano(), now.UnixNano()},
		},
		{
			name: "with only start time",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   now.Add(-1 * time.Hour),
			},
			expectedSQL:  sql.SelectSpansByTraceID + "\n\tAND s.start_time >= fromUnixTimestamp64Nano(?)",
			expectedArgs: []any{traceID, now.Add(-1 * time.Hour).UnixNano()},
		},
		{
			name: "with only end time",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
				End:     now,
			},
			expectedSQL:  sql.SelectSpansByTraceID + "\n\tAND s.start_time <= fromUnixTimestamp64Nano(?)",
			expectedArgs: []any{traceID, now.UnixNano()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args := buildGetTracesQuery(tt.params)
			require.Equal(t, tt.expectedSQL, query)
			require.Equal(t, tt.expectedArgs, args)
		})
	}
}

func TestBuildStringAttributeCondition_MultipleTypes(t *testing.T) {
	attr := pcommon.NewValueStr("123") // parses as both int and str
	var q sqlBuilder
	var args []any

	metadata := attributeMetadata{
		"http.status": {span: []pcommon.ValueType{pcommon.ValueTypeInt, pcommon.ValueTypeStr}},
	}

	args = q.buildStringAttributeCondition(args, "http.status", attr, metadata)

	query := q.String()
	assert.Contains(t, query, "int_attributes")
	assert.Contains(t, query, "OR")
	assert.Contains(t, query, "str_attributes")
	assert.Len(t, args, 4)
}

// TestBuildStringAttributeCondition_AllLevels gives every level a different stored type, so
// the expected SQL pins both the order of the levels and the column each level reads.
func TestBuildStringAttributeCondition_AllLevels(t *testing.T) {
	attr := pcommon.NewValueStr("1") // parses as every scalar type
	metadata := attributeMetadata{
		"k": {
			resource: []pcommon.ValueType{pcommon.ValueTypeInt},
			scope:    []pcommon.ValueType{pcommon.ValueTypeDouble},
			span:     []pcommon.ValueType{pcommon.ValueTypeBool},
			event:    []pcommon.ValueType{pcommon.ValueTypeStr},
			link:     []pcommon.ValueType{pcommon.ValueTypeInt},
		},
	}
	var q sqlBuilder

	args := q.buildStringAttributeCondition(nil, "k", attr, metadata)

	const expectedSQL = "\n\t\tarrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)" +
		"\n\t\tOR" +
		"\n\t\tarrayExists((key, value) -> key = ? AND value = ?, s.scope_double_attributes.key, s.scope_double_attributes.value)" +
		"\n\t\tOR" +
		"\n\t\tarrayExists((key, value) -> key = ? AND value = ?, s.bool_attributes.key, s.bool_attributes.value)" +
		"\n\t\tOR" +
		"\n\t\tarrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.str_attributes.key, x.str_attributes.value), s.events)" +
		"\n\t\tOR" +
		"\n\t\tarrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x.int_attributes.key, x.int_attributes.value), s.links)"
	require.Equal(t, expectedSQL, q.String())
	require.Equal(t, []any{"k", int64(1), "k", float64(1), "k", true, "k", "1", "k", int64(1)}, args)
}

func TestBuildFindTracesQuery(t *testing.T) {
	start := time.Unix(1700000000, 123456789)
	end := start.Add(3*time.Second + 987654321*time.Nanosecond)
	const boundedTrace = "(s.trace_id = ? AND s.start_time >= fromUnixTimestamp64Nano(?) AND s.start_time <= fromUnixTimestamp64Nano(?))"
	for _, tc := range []struct {
		name      string
		bounds    []tracestore.FoundTraceID
		condition string
		args      []any
	}{
		{
			name:      "single trace",
			bounds:    []tracestore.FoundTraceID{{TraceID: pcommon.TraceID{15: 1}, Start: start, End: end}},
			condition: boundedTrace,
			args:      []any{traceIDHex1, start.UnixNano(), end.UnixNano()},
		},
		{
			name: "long trace does not widen another trace's range",
			bounds: []tracestore.FoundTraceID{
				{TraceID: pcommon.TraceID{15: 1}, Start: start.Add(time.Second), End: end.Add(-time.Second)},
				{TraceID: pcommon.TraceID{15: 2}, Start: start.Add(-24 * time.Hour), End: end.Add(24 * time.Hour)},
			},
			condition: boundedTrace + " OR " + boundedTrace,
			args: []any{
				traceIDHex1, start.Add(time.Second).UnixNano(), end.Add(-time.Second).UnixNano(),
				traceIDHex2, start.Add(-24 * time.Hour).UnixNano(), end.Add(24 * time.Hour).UnixNano(),
			},
		},
		{
			name: "mixed metadata",
			bounds: []tracestore.FoundTraceID{
				{TraceID: pcommon.TraceID{15: 1}, Start: start, End: end},
				{TraceID: pcommon.TraceID{15: 2}},
				{TraceID: pcommon.TraceID{15: 3}, Start: start.Add(-time.Hour), End: end.Add(time.Hour)},
			},
			condition: boundedTrace + " OR (s.trace_id = ?) OR " + boundedTrace,
			args: []any{
				traceIDHex1, start.UnixNano(), end.UnixNano(), traceIDHex2,
				"00000000000000000000000000000003", start.Add(-time.Hour).UnixNano(), end.Add(time.Hour).UnixNano(),
			},
		},
		{
			name:      "missing start",
			bounds:    []tracestore.FoundTraceID{{TraceID: pcommon.TraceID{15: 1}, End: end}},
			condition: "(s.trace_id = ?)",
			args:      []any{traceIDHex1},
		},
		{
			name:      "missing end",
			bounds:    []tracestore.FoundTraceID{{TraceID: pcommon.TraceID{15: 1}, Start: start}},
			condition: "(s.trace_id = ?)",
			args:      []any{traceIDHex1},
		},
		{
			name: "all metadata missing",
			bounds: []tracestore.FoundTraceID{
				{TraceID: pcommon.TraceID{15: 1}}, {TraceID: pcommon.TraceID{15: 2}},
			},
			condition: "(s.trace_id = ?) OR (s.trace_id = ?)",
			args:      []any{traceIDHex1, traceIDHex2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, args := buildFindTracesQuery(tc.bounds)
			require.Equal(t, strings.TrimRight(sql.SelectSpansQuery, "\n")+"\nWHERE "+tc.condition+"\nORDER BY s.trace_id", q)
			require.Equal(t, tc.args, args)
		})
	}
}
