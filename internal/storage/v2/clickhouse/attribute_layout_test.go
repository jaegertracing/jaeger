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
		// columnTypes is the rows of the lookup: none when the spans table does not exist.
		columnTypes []string
		queryErr    error
		scanErr     error
		rowsErr     error
		wantErr     string
	}{
		{
			name:        "nested table and nested configuration",
			configured:  chtracestore.NestedLayout{},
			columnTypes: []string{"Array(String)"},
		},
		{
			name:        "nested table without flatten_nested",
			configured:  chtracestore.NestedLayout{},
			columnTypes: []string{"Nested(key String, value String)"},
		},
		{
			name:        "map table and map configuration",
			configured:  chtracestore.MapLayout{},
			columnTypes: []string{"Map(String, String)"},
		},
		{
			name:       "no table",
			configured: chtracestore.MapLayout{},
		},
		{
			name:        "map table and nested configuration",
			configured:  chtracestore.NestedLayout{},
			columnTypes: []string{"Map(String, String)"},
			wantErr: "the spans table declares its string attributes as Map(String, String), which is not the nested layout that attribute_layout selects; " +
				"an existing table is not converted, so change attribute_layout or create the tables in a new database",
		},
		{
			name:        "nested table and map configuration",
			configured:  chtracestore.MapLayout{},
			columnTypes: []string{"Array(String)"},
			wantErr:     "the spans table declares its string attributes as Array(String), which is not the map layout",
		},
		{
			name:       "query fails",
			configured: chtracestore.NestedLayout{},
			queryErr:   assert.AnError,
			wantErr:    "failed to read the attribute layout of the spans table",
		},
		{
			name:        "scan fails",
			configured:  chtracestore.NestedLayout{},
			columnTypes: []string{"Array(String)"},
			scanErr:     assert.AnError,
			wantErr:     "failed to read the attribute layout of the spans table",
		},
		{
			name:       "rows fail",
			configured: chtracestore.NestedLayout{},
			rowsErr:    assert.AnError,
			wantErr:    "failed to read the attribute layout of the spans table",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					"FROM system.columns": {
						Rows: &clickhousetest.Rows[string]{
							Data:    tt.columnTypes,
							ScanErr: tt.scanErr,
							RowsErr: tt.rowsErr,
							ScanFn: func(dest any, src string) error {
								*dest.([]any)[0].(*string) = src
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
