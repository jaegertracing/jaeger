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
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
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
			var q strings.Builder
			var args []any

			args = buildStringAttributeCondition(&q, args, "k", attr, tc.metadata)

			query := q.String()
			assert.Contains(t, query, "str_attributes")
			assert.Contains(t, query, "resource_str_attributes")
			assert.Contains(t, query, "scope_str_attributes")
			assert.Contains(t, query, "events")
			assert.Contains(t, query, "links")
			assert.Len(t, args, 10)
		})
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

func TestBuildFindTraceIDsQuery_ErrorAttribute(t *testing.T) {
	tests := []struct {
		name     string
		setAttr  func(attrs pcommon.Map)
		wantCond string
		wantArgs []any
	}{
		{
			name:     "true as string",
			setAttr:  func(attrs pcommon.Map) { attrs.PutStr("error", "true") },
			wantCond: "s.status_code = ?",
			wantArgs: []any{"svc", "Error", 10},
		},
		{
			name:     "true as bool",
			setAttr:  func(attrs pcommon.Map) { attrs.PutBool("error", true) },
			wantCond: "s.status_code = ?",
			wantArgs: []any{"svc", "Error", 10},
		},
		{
			name:     "false as string",
			setAttr:  func(attrs pcommon.Map) { attrs.PutStr("error", "false") },
			wantCond: "s.status_code != ?",
			wantArgs: []any{"svc", "Error", 10},
		},
		{
			name:     "false as bool",
			setAttr:  func(attrs pcommon.Map) { attrs.PutBool("error", false) },
			wantCond: "s.status_code != ?",
			wantArgs: []any{"svc", "Error", 10},
		},
		{
			name:     "invalid value matches nothing",
			setAttr:  func(attrs pcommon.Map) { attrs.PutInt("error", 1) },
			wantCond: "1 = 0",
			wantArgs: []any{"svc", 10},
		},
		{
			name:     "unparseable string matches nothing",
			setAttr:  func(attrs pcommon.Map) { attrs.PutStr("error", "maybe") },
			wantCond: "1 = 0",
			wantArgs: []any{"svc", 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			tt.setAttr(attrs)

			// The bare driver proves no attribute metadata query is issued.
			reader := NewReader(&clickhousetest.Driver{}, testReaderConfig)
			query, args, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{
				ServiceName: "svc",
				Attributes:  attrs,
				SearchDepth: 10,
			})
			require.NoError(t, err)

			assert.Contains(t, query, tt.wantCond)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestBuildFindTraceIDsQuery_ErrorAttributeAlongsideOthers(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("error", "true")
	attrs.PutStr("http.method", "GET")

	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data:   nil,
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	query, args, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{
		ServiceName: "svc",
		Attributes:  attrs,
		SearchDepth: 10,
	})
	require.NoError(t, err)

	assert.Contains(t, query, "s.status_code = ?")
	assert.Contains(t, query, "arrayExists")
	assert.Equal(t, []any{
		"svc", "Error",
		"http.method", "GET", "http.method", "GET", "http.method", "GET",
		"http.method", "GET", "http.method", "GET",
		10,
	}, args)
}

func TestBuildFindTraceIDsQuery_ErrorFalseAlongsideOthers(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutBool("error", false)
	attrs.PutStr("http.method", "GET")

	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data:   nil,
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	query, args, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{
		ServiceName: "svc",
		Attributes:  attrs,
		SearchDepth: 10,
	})
	require.NoError(t, err)

	assert.Contains(t, query, "s.status_code != ?")
	assert.Contains(t, query, "arrayExists")
	assert.Equal(t, []any{
		"svc", "Error",
		"http.method", "GET", "http.method", "GET", "http.method", "GET",
		"http.method", "GET", "http.method", "GET",
		10,
	}, args)
}

func TestBuildFindTraceIDsQuery_InvalidErrorDropsSiblings(t *testing.T) {
	// An unparseable error value matches nothing, so the sibling attribute
	// conditions are intentionally dropped.
	attrs := pcommon.NewMap()
	attrs.PutStr("error", "maybe")
	attrs.PutStr("http.method", "GET")

	reader := NewReader(&clickhousetest.Driver{}, testReaderConfig)
	query, args, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{
		ServiceName: "svc",
		Attributes:  attrs,
		SearchDepth: 10,
	})
	require.NoError(t, err)

	assert.Contains(t, query, "1 = 0")
	assert.NotContains(t, query, "arrayExists")
	assert.Equal(t, []any{"svc", 10}, args)
}
