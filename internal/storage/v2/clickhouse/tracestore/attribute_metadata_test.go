// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

func TestGetAttributeMetadata_ErrorCases(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	tests := []struct {
		name        string
		driver      *testDriver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: nil,
						err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query attribute metadata",
		},
		{
			name: "ScanStructError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: &testRows[dbmodel.AttributeMetadata]{
							data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "span",
							}},
							scanErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectedErr: "failed to scan row",
		},
		{
			name: "RowsIterationError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: &testRows[dbmodel.AttributeMetadata]{
							data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "span",
							}},
							scanFn: func(dest any, src dbmodel.AttributeMetadata) error {
								ptr, ok := dest.(*dbmodel.AttributeMetadata)
								if !ok {
									return assert.AnError
								}
								*ptr = src
								return nil
							},
							rowsErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectedErr: "error iterating attribute metadata rows",
		},
		{
			name: "UnknownLevelError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: &testRows[dbmodel.AttributeMetadata]{
							data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "unknown",
							}},
							scanFn: func(dest any, src dbmodel.AttributeMetadata) error {
								ptr, ok := dest.(*dbmodel.AttributeMetadata)
								if !ok {
									return assert.AnError
								}
								*ptr = src
								return nil
							},
						},
						err: nil,
					},
				},
			},
			expectedErr: "unknown attribute level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewReader(tt.driver, ReaderConfig{})
			_, err := reader.getAttributeMetadata(t.Context(), attrs)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.expectedErr)
		})
	}
}
