// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
)

func TestCheckAttributeLayout(t *testing.T) {
	tests := []struct {
		name       string
		configured chtracestore.AttributeLayout
		// isMap is the rows of the lookup: none when the spans table does not exist.
		isMap    []uint8
		queryErr error
		scanErr  error
		rowsErr  error
		wantErr  string
	}{
		{
			name:       "nested table and nested configuration",
			configured: chtracestore.AttributeLayoutNested,
			isMap:      []uint8{0},
		},
		{
			name:       "map table and map configuration",
			configured: chtracestore.AttributeLayoutMap,
			isMap:      []uint8{1},
		},
		{
			name:       "no table",
			configured: chtracestore.AttributeLayoutMap,
		},
		{
			name:       "map table and nested configuration",
			configured: chtracestore.AttributeLayoutNested,
			isMap:      []uint8{1},
			wantErr: "the spans table stores attributes in the map layout, and attribute_layout is nested; " +
				"an existing table is not converted, so set attribute_layout to map or create the tables in a new database",
		},
		{
			name:       "nested table and map configuration",
			configured: chtracestore.AttributeLayoutMap,
			isMap:      []uint8{0},
			wantErr:    "the spans table stores attributes in the nested layout, and attribute_layout is map",
		},
		{
			name:       "query fails",
			configured: chtracestore.AttributeLayoutNested,
			queryErr:   assert.AnError,
			wantErr:    "failed to read the attribute layout of the spans table",
		},
		{
			name:       "scan fails",
			configured: chtracestore.AttributeLayoutNested,
			isMap:      []uint8{0},
			scanErr:    assert.AnError,
			wantErr:    "failed to read the attribute layout of the spans table",
		},
		{
			name:       "rows fail",
			configured: chtracestore.AttributeLayoutNested,
			rowsErr:    assert.AnError,
			wantErr:    "failed to read the attribute layout of the spans table",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					"FROM system.columns": {
						Rows: &clickhousetest.Rows[uint8]{
							Data:    tt.isMap,
							ScanErr: tt.scanErr,
							RowsErr: tt.rowsErr,
							ScanFn: func(dest any, src uint8) error {
								*dest.([]any)[0].(*uint8) = src
								return nil
							},
						},
						Err: tt.queryErr,
					},
				},
			}

			err := checkAttributeLayout(t.Context(), conn, tt.configured)

			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
