// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
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

		reader := NewReader(&testDriver{t: t}, testReaderConfig)
		_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: attrs})

		require.Error(t, err)
		require.ErrorContains(t, err, "failed to marshal slice attribute")
	})

	t.Run("marshal map error", func(t *testing.T) {
		attrs := pcommon.NewMap()
		m := attrs.PutEmptyMap("bad_map")
		m.PutEmpty("key")

		reader := NewReader(&testDriver{t: t}, testReaderConfig)
		_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: attrs})

		require.Error(t, err)
		require.ErrorContains(t, err, "failed to marshal map attribute")
	})
}

func TestBuildFindTraceIDsQuery_AttributeMetadataError(t *testing.T) {
	td := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectAttributeMetadata: {
				rows: nil,
				err:  assert.AnError,
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
			assert.Len(t, args, 8)
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
