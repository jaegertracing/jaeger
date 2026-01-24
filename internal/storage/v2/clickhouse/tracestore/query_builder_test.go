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

func TestBuildStringAttributeCondition_Errors(t *testing.T) {
	cases := []struct {
		name        string
		attrValue   string
		metadata    attributeMetadata
		expectedErr string
	}{
		{
			name:      "parse bool fails",
			attrValue: "not-bool",
			metadata: attributeMetadata{
				"k": {span: []string{"bool"}},
			},
			expectedErr: "failed to parse bool attribute",
		},
		{
			name:      "parse double fails",
			attrValue: "not-float",
			metadata: attributeMetadata{
				"k": {span: []string{"double"}},
			},
			expectedErr: "failed to parse double attribute",
		},
		{
			name:      "parse int fails",
			attrValue: "not-int",
			metadata: attributeMetadata{
				"k": {span: []string{"int"}},
			},
			expectedErr: "failed to parse int attribute",
		},
		{
			name:      "parse fails at resource level",
			attrValue: "not-bool",
			metadata: attributeMetadata{
				"k": {resource: []string{"bool"}},
			},
			expectedErr: "failed to parse bool attribute",
		},
		{
			name:      "parse fails at scope level",
			attrValue: "not-int",
			metadata: attributeMetadata{
				"k": {scope: []string{"int"}},
			},
			expectedErr: "failed to parse int attribute",
		},
		{
			name:      "unsupported type",
			attrValue: "whatever",
			metadata: attributeMetadata{
				"k": {span: []string{"unknown"}},
			},
			expectedErr: "unsupported attribute type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attr := pcommon.NewValueStr(tc.attrValue)
			var q strings.Builder
			var args []any

			err := buildStringAttributeCondition(&q, &args, "k", attr, tc.metadata)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.expectedErr)
		})
	}
}
