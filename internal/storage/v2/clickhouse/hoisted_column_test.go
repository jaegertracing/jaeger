// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
)

func TestNewHoistedColumns(t *testing.T) {
	validAttribute := HoistedAttribute{
		Key:    "gen_ai.request.model",
		Level:  "span",
		Type:   "string",
		Column: "gen_ai_request_model",
	}
	validWith := func(mutate func(attribute *HoistedAttribute)) []HoistedAttribute {
		attribute := validAttribute
		mutate(&attribute)
		return []HoistedAttribute{attribute}
	}
	tests := []struct {
		name       string
		attributes []HoistedAttribute
		wantErr    string
	}{
		{
			name:       "one entry",
			attributes: []HoistedAttribute{validAttribute},
		},
		{
			name: "same key at two types and two levels",
			attributes: []HoistedAttribute{
				validAttribute,
				{
					Key:    validAttribute.Key,
					Level:  "span",
					Type:   "int",
					Column: "model_int",
				},
				{
					Key:    validAttribute.Key,
					Level:  "resource",
					Type:   "string",
					Column: "resource_model",
					Codec:  "ZSTD(22)",
				},
			},
		},
		{
			name:       "empty key",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Key = "" }),
			wantErr:    "hoisted_attributes[0]: key must not be empty",
		},
		{
			name:       "unsupported level",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Level = "scope" }),
			wantErr:    `level must be span or resource, not "scope"`,
		},
		{
			name:       "unsupported type",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Type = "bytes" }),
			wantErr:    `type must be string, int, double or bool, not "bytes"`,
		},
		{
			name:       "column is not an identifier",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "model; DROP TABLE spans" }),
			wantErr:    "must match [a-z][a-z0-9_]*",
		},
		{
			name:       "column that starts like a virtual column",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "_row_exists" }),
			wantErr:    "must match [a-z][a-z0-9_]*",
		},
		{
			name:       "empty column",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "" }),
			wantErr:    "must match [a-z][a-z0-9_]*",
		},
		{
			name:       "column of the spans table",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "trace_id" }),
			wantErr:    `column "trace_id" is already a column`,
		},
		{
			name:       "attribute group of the spans table",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Column = "resource_str_attributes" }),
			wantErr:    `column "resource_str_attributes" is already a column`,
		},
		{
			name: "column used twice",
			attributes: []HoistedAttribute{
				validAttribute,
				{
					Key:    "other",
					Level:  "span",
					Type:   "string",
					Column: validAttribute.Column,
				},
			},
			wantErr: `hoisted_attributes[1]: column "gen_ai_request_model" is already a column`,
		},
		{
			name: "attribute listed twice",
			attributes: []HoistedAttribute{
				validAttribute,
				{
					Key:    validAttribute.Key,
					Level:  validAttribute.Level,
					Type:   validAttribute.Type,
					Column: "second_copy",
				},
			},
			wantErr: `the span attribute "gen_ai.request.model" of type string is listed twice`,
		},
		{
			name:       "codec other than ZSTD",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "LZ4" }),
			wantErr:    `codec "LZ4" must be ZSTD(level)`,
		},
		{
			name:       "codec with SQL after it",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "ZSTD(1)) TTL now(" }),
			wantErr:    "must be ZSTD(level)",
		},
		{
			name:       "codec level zero",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "ZSTD(0)" }),
			wantErr:    "must be ZSTD(level)",
		},
		{
			name:       "codec level above the maximum",
			attributes: validWith(func(attribute *HoistedAttribute) { attribute.Codec = "ZSTD(23)" }),
			wantErr:    "must be ZSTD(level)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHoistedColumns(tt.attributes, chtracestore.NestedLayout{})
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

// TestNewHoistedColumns_SpansTableColumns pins the columns of the spans table that a hoisted
// column must not take. The names are read from the table template, so a change in the layout
// of the template that hid a column from that scan would fail here.
func TestNewHoistedColumns_SpansTableColumns(t *testing.T) {
	spansColumns := []string{
		"id",
		"trace_id",
		"trace_state",
		"parent_span_id",
		"name",
		"kind",
		"start_time",
		"status_code",
		"status_message",
		"duration",
		"bool_attributes",
		"double_attributes",
		"int_attributes",
		"str_attributes",
		"complex_attributes",
		"events",
		"links",
		"service_name",
		"resource_bool_attributes",
		"resource_double_attributes",
		"resource_int_attributes",
		"resource_str_attributes",
		"resource_complex_attributes",
		"scope_name",
		"scope_version",
		"scope_bool_attributes",
		"scope_double_attributes",
		"scope_int_attributes",
		"scope_str_attributes",
		"scope_complex_attributes",
	}
	for _, column := range spansColumns {
		_, err := newHoistedColumns([]HoistedAttribute{
			{
				Key:    "k",
				Level:  "span",
				Type:   "string",
				Column: column,
			},
		}, chtracestore.NestedLayout{})
		require.ErrorContains(t, err, "is already a column of the spans table", column)
	}
	assert.Len(t, spansColumnPattern.FindAllStringSubmatch(sql.CreateSpansTable, -1), len(spansColumns))
}

// tableEngine is one row of the lookup of the table engines.
type tableEngine struct {
	name   string
	engine string
}

// hoistedColumnConn exists because the column lookups of the two tables send the same query
// text, by which the shared test driver selects a response. It answers the lookup of the
// table engines from the tables field, and a column lookup by the table name in the query
// arguments.
type hoistedColumnConn struct {
	driver.Conn

	tables         []tableEngine
	tableLookupErr error
	tableScanErr   error
	tableRowsErr   error
	// columnStateByTable holds how each table declares the column; a table without an
	// entry does not have the column.
	columnStateByTable map[string]hoistedColumnState
	columnLookupErr    error
	columnScanErr      error
	columnRowsErr      error

	// tableLookupArgs holds the arguments of the lookup of the table engines.
	tableLookupArgs []any
	// columnLookupArgs holds the arguments of every column lookup, in call order.
	columnLookupArgs [][]any
}

func (c *hoistedColumnConn) Query(_ context.Context, query string, args ...any) (driver.Rows, error) {
	if strings.Contains(query, "FROM system.tables") {
		c.tableLookupArgs = args
		return &clickhousetest.Rows[tableEngine]{
			Data:    c.tables,
			ScanErr: c.tableScanErr,
			RowsErr: c.tableRowsErr,
			ScanFn: func(dest any, src tableEngine) error {
				fields := dest.([]any)
				*fields[0].(*string) = src.name
				*fields[1].(*string) = src.engine
				return nil
			},
		}, c.tableLookupErr
	}
	c.columnLookupArgs = append(c.columnLookupArgs, args)
	rows := &clickhousetest.Rows[hoistedColumnState]{
		ScanErr: c.columnScanErr,
		RowsErr: c.columnRowsErr,
		ScanFn: func(dest any, src hoistedColumnState) error {
			*dest.(*hoistedColumnState) = src
			return nil
		},
	}
	if state, ok := c.columnStateByTable[args[1].(string)]; ok {
		rows.Data = []hoistedColumnState{state}
	}
	return rows, c.columnLookupErr
}

func TestCheckHoistedColumns(t *testing.T) {
	columns, err := newHoistedColumns([]HoistedAttribute{
		{
			Key:    "gen_ai.usage.input_tokens",
			Level:  "span",
			Type:   "int",
			Column: "input_tokens",
		},
	}, chtracestore.NestedLayout{})
	require.NoError(t, err)
	matchingState := hoistedColumnState{
		Type:           "Nullable(Int64)",
		Kind:           "MATERIALIZED",
		Expression:     columns[0].SourceExpression(),
		Codec:          "CODEC(ZSTD(1))",
		SameExpression: 1,
	}
	stateWith := func(mutate func(state *hoistedColumnState)) hoistedColumnState {
		state := matchingState
		mutate(&state)
		return state
	}
	stateWithAnotherCodec := stateWith(func(state *hoistedColumnState) { state.Codec = "CODEC(ZSTD(3))" })
	nonDistributedTables := []tableEngine{
		{name: "spans", engine: "MergeTree"},
	}
	distributedTables := []tableEngine{
		{name: "spans", engine: "Distributed"},
		{name: "spans_local", engine: "ReplicatedMergeTree"},
	}
	nonDistributedConn := func(state hoistedColumnState) *hoistedColumnConn {
		return &hoistedColumnConn{
			tables:             nonDistributedTables,
			columnStateByTable: map[string]hoistedColumnState{"spans": state},
		}
	}
	tests := []struct {
		name string
		conn *hoistedColumnConn
		// wantCheckedTables holds the tables whose column the check reads, in order.
		wantCheckedTables []string
	}{
		{
			name:              "non-Distributed table, column matches",
			conn:              nonDistributedConn(matchingState),
			wantCheckedTables: []string{"spans"},
		},
		{
			name:              "non-Distributed table, column is missing",
			conn:              &hoistedColumnConn{tables: nonDistributedTables},
			wantCheckedTables: []string{"spans"},
		},
		{
			name:              "non-Distributed table, another type",
			conn:              nonDistributedConn(stateWith(func(state *hoistedColumnState) { state.Type = "Nullable(String)" })),
			wantCheckedTables: []string{"spans"},
		},
		{
			name: "non-Distributed table, an ordinary column",
			conn: nonDistributedConn(stateWith(func(state *hoistedColumnState) {
				state.Kind = ""
				state.Expression = ""
				state.SameExpression = 0
			})),
			wantCheckedTables: []string{"spans"},
		},
		{
			name:              "non-Distributed table, a default in place of a materialized column",
			conn:              nonDistributedConn(stateWith(func(state *hoistedColumnState) { state.Kind = "DEFAULT" })),
			wantCheckedTables: []string{"spans"},
		},
		{
			name: "non-Distributed table, another expression",
			conn: nonDistributedConn(stateWith(func(state *hoistedColumnState) {
				state.Expression = "if(has(int_attributes.key, 'another.key'), int_attributes.value[indexOf(int_attributes.key, 'another.key')], NULL)"
				state.SameExpression = 0
			})),
			wantCheckedTables: []string{"spans"},
		},
		{
			name:              "non-Distributed table, another codec",
			conn:              nonDistributedConn(stateWithAnotherCodec),
			wantCheckedTables: []string{"spans"},
		},
		{
			name: "non-Distributed table beside an unrelated spans_local, column is missing",
			conn: &hoistedColumnConn{
				tables: []tableEngine{
					{name: "spans", engine: "MergeTree"},
					{name: "spans_local", engine: "MergeTree"},
				},
			},
			wantCheckedTables: []string{"spans"},
		},
		{
			name: "Distributed table, both tables match",
			conn: &hoistedColumnConn{
				tables: distributedTables,
				columnStateByTable: map[string]hoistedColumnState{
					"spans_local": matchingState,
					"spans":       matchingState,
				},
			},
			wantCheckedTables: []string{"spans_local", "spans"},
		},
		{
			name: "Distributed table, only the Distributed table has the column",
			conn: &hoistedColumnConn{
				tables: distributedTables,
				columnStateByTable: map[string]hoistedColumnState{
					"spans": matchingState,
				},
			},
			wantCheckedTables: []string{"spans_local", "spans"},
		},
		{
			name: "Distributed table, only the storage table has the column",
			conn: &hoistedColumnConn{
				tables: distributedTables,
				columnStateByTable: map[string]hoistedColumnState{
					"spans_local": matchingState,
				},
			},
			wantCheckedTables: []string{"spans_local", "spans"},
		},
		{
			name:              "Distributed table, neither table has the column",
			conn:              &hoistedColumnConn{tables: distributedTables},
			wantCheckedTables: []string{"spans_local", "spans"},
		},
		{
			name: "Distributed table, the storage table declares another codec",
			conn: &hoistedColumnConn{
				tables: distributedTables,
				columnStateByTable: map[string]hoistedColumnState{
					"spans_local": stateWithAnotherCodec,
					"spans":       matchingState,
				},
			},
			wantCheckedTables: []string{"spans_local", "spans"},
		},
		{
			name: "Distributed table, both tables declare another codec",
			conn: &hoistedColumnConn{
				tables: distributedTables,
				columnStateByTable: map[string]hoistedColumnState{
					"spans_local": stateWithAnotherCodec,
					"spans":       stateWithAnotherCodec,
				},
			},
			wantCheckedTables: []string{"spans_local", "spans"},
		},
		{
			name: "Distributed table, the storage table lacks the column and the Distributed table declares another codec",
			conn: &hoistedColumnConn{
				tables: distributedTables,
				columnStateByTable: map[string]hoistedColumnState{
					"spans": stateWithAnotherCodec,
				},
			},
			wantCheckedTables: []string{"spans_local", "spans"},
		},
		{
			name: "Distributed table without a spans_local table, column is missing",
			conn: &hoistedColumnConn{
				tables: []tableEngine{
					{name: "spans", engine: "Distributed"},
				},
			},
			wantCheckedTables: []string{"spans"},
		},
		{
			name: "spans_local without spans",
			conn: &hoistedColumnConn{
				tables: []tableEngine{
					{name: "spans_local", engine: "MergeTree"},
				},
			},
		},
		{
			name: "no tables",
			conn: &hoistedColumnConn{},
		},
		{
			name: "table lookup fails",
			conn: &hoistedColumnConn{tableLookupErr: assert.AnError},
		},
		{
			name: "table lookup scan fails",
			conn: &hoistedColumnConn{
				tables:       nonDistributedTables,
				tableScanErr: assert.AnError,
			},
		},
		{
			name: "table lookup rows fail",
			conn: &hoistedColumnConn{tableRowsErr: assert.AnError},
		},
		{
			name: "column lookup fails",
			conn: &hoistedColumnConn{
				tables:          nonDistributedTables,
				columnLookupErr: assert.AnError,
			},
			wantCheckedTables: []string{"spans"},
		},
		{
			name: "column lookup scan fails",
			conn: &hoistedColumnConn{
				tables:             nonDistributedTables,
				columnStateByTable: map[string]hoistedColumnState{"spans": matchingState},
				columnScanErr:      assert.AnError,
			},
			wantCheckedTables: []string{"spans"},
		},
		{
			name: "column lookup rows fail",
			conn: &hoistedColumnConn{
				tables:        nonDistributedTables,
				columnRowsErr: assert.AnError,
			},
			wantCheckedTables: []string{"spans"},
		},
	}
	var errorsByCase strings.Builder
	for _, tt := range tests {
		err := checkHoistedColumns(t.Context(), tt.conn, columns)

		message := "no error"
		if err != nil {
			message = err.Error()
		}
		fmt.Fprintf(&errorsByCase, "-- %s\n%s\n\n", tt.name, message)
		assert.Equal(t, []any{"spans", "spans_local"}, tt.conn.tableLookupArgs, tt.name)
		// The server compares the stored expression with the one that the configuration
		// renders, and looks the column up by its table and its name.
		var wantColumnLookupArgs [][]any
		for _, table := range tt.wantCheckedTables {
			wantColumnLookupArgs = append(wantColumnLookupArgs, []any{columns[0].SourceExpression(), table, "input_tokens"})
		}
		assert.Equal(t, wantColumnLookupArgs, tt.conn.columnLookupArgs, tt.name)
	}
	clickhousetest.VerifyQuerySnapshot(t, errorsByCase.String())
}

func TestCheckHoistedColumns_NoColumns(t *testing.T) {
	conn := &clickhousetest.Driver{}

	require.NoError(t, checkHoistedColumns(t.Context(), conn, nil))

	assert.Empty(t, conn.RecordedQueries)
}
