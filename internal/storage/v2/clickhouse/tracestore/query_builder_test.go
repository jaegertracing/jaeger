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
		// additional typed columns that should appear in the fallback (beyond the always-present str/bytes/map/slice)
		extraParsableTypes []pcommon.ValueType
	}{
		{
			name:               "parse bool fails falls back to str",
			attrValue:          "not-bool",
			metadata:           attributeMetadata{"k": {span: []pcommon.ValueType{pcommon.ValueTypeBool}}},
			extraParsableTypes: nil,
		},
		{
			name:               "parse double fails falls back to str",
			attrValue:          "not-float",
			metadata:           attributeMetadata{"k": {span: []pcommon.ValueType{pcommon.ValueTypeDouble}}},
			extraParsableTypes: nil,
		},
		{
			name:               "parse int fails falls back to str",
			attrValue:          "not-int",
			metadata:           attributeMetadata{"k": {span: []pcommon.ValueType{pcommon.ValueTypeInt}}},
			extraParsableTypes: nil,
		},
		{
			name:               "unsupported type falls back to str",
			attrValue:          "whatever",
			metadata:           attributeMetadata{"k": {span: []pcommon.ValueType{pcommon.ValueTypeEmpty}}},
			extraParsableTypes: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attr := pcommon.NewValueStr(tc.attrValue)
			var q strings.Builder
			var args []any

			args = buildStringAttributeCondition(&q, args, "k", attr, tc.metadata)

			query := q.String()
			assert.Contains(t, query, "str_attributes")
			assert.Contains(t, query, "resource_str_attributes")
			assert.Contains(t, query, "scope_str_attributes")
			assert.Contains(t, query, "events")
			assert.Contains(t, query, "links")
			// str always parses; bool/int/double only if value is parseable
			alwaysTypes := 1 // str
			extraTypes := len(tc.extraParsableTypes)
			assert.Len(t, args, (alwaysTypes+extraTypes)*5*2)
		})
	}
}

func TestBuildStringAttributeCondition_FallbackWithNumericValue(t *testing.T) {
	// When metadata is empty and value is numeric "123", the fallback should
	// generate conditions for str, int, double (3 types)
	attr := pcommon.NewValueStr("123")
	var q strings.Builder
	var args []any

	args = buildStringAttributeCondition(&q, args, "k", attr, attributeMetadata{})

	query := q.String()
	assert.Contains(t, query, "str_attributes")
	assert.Contains(t, query, "int_attributes")
	assert.Contains(t, query, "double_attributes")
	assert.NotContains(t, query, "complex_attributes")
	assert.Len(t, args, 30) // 3 types × 5 levels × 2 args
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
			expectedSQL:  sql.SelectSpansByTraceID + " AND s.start_time >= ? AND s.start_time <= ?",
			expectedArgs: []any{traceID, now.Add(-1 * time.Hour), now},
		},
		{
			name: "with only start time",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   now.Add(-1 * time.Hour),
			},
			expectedSQL:  sql.SelectSpansByTraceID + " AND s.start_time >= ?",
			expectedArgs: []any{traceID, now.Add(-1 * time.Hour)},
		},
		{
			name: "with only end time",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
				End:     now,
			},
			expectedSQL:  sql.SelectSpansByTraceID + " AND s.start_time <= ?",
			expectedArgs: []any{traceID, now},
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
	var q strings.Builder
	var args []any

	metadata := attributeMetadata{
		"http.status": {span: []pcommon.ValueType{pcommon.ValueTypeInt, pcommon.ValueTypeStr}},
	}

	args = buildStringAttributeCondition(&q, args, "http.status", attr, metadata)

	query := q.String()
	assert.Contains(t, query, "int_attributes")
	assert.Contains(t, query, "OR")
	assert.Contains(t, query, "str_attributes")
	assert.Len(t, args, 4)
}
