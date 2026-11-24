// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
)

const (
	defaultHoistedCodec = "ZSTD(1)"
	spansTable          = "spans"
)

var (
	hoistedColumnNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	hoistedCodecPattern      = regexp.MustCompile(`^ZSTD\((\d{1,2})\)$`)
	// spansColumnPattern matches the name of a top-level column in the spans table template,
	// which is how a hoisted column is checked against the columns the table already has.
	spansColumnPattern = regexp.MustCompile(`(?m)^ {8}([a-z_]+) `)

	hoistedLevels = map[string]expression.Level{
		"span":     expression.LevelSpan,
		"resource": expression.LevelResource,
	}
	hoistedTypes = map[string]pcommon.ValueType{
		"string": pcommon.ValueTypeStr,
		"int":    pcommon.ValueTypeInt,
		"double": pcommon.ValueTypeDouble,
		"bool":   pcommon.ValueTypeBool,
	}
)

// zstdCodec returns codec as ClickHouse stores it, and false unless codec is ZSTD(level) with
// a level that ClickHouse accepts. No other codec is accepted, so that the value can be
// rendered into DDL without being arbitrary SQL. The level is rendered from its number
// because ClickHouse stores ZSTD(01) as ZSTD(1).
func zstdCodec(codec string) (string, bool) {
	match := hoistedCodecPattern.FindStringSubmatch(codec)
	if match == nil {
		return "", false
	}
	// The pattern admits one or two digits, so the conversion cannot fail.
	level, _ := strconv.Atoi(match[1])
	if level < 1 || level > 22 {
		return "", false
	}
	return fmt.Sprintf("ZSTD(%d)", level), true
}

// newHoistedColumns validates attributes and returns the columns they describe, in the
// configured order. Validation and conversion share one function so that no column can be
// rendered from an entry that was not checked.
func newHoistedColumns(attributes []HoistedAttribute, layout chtracestore.AttributeLayout) ([]chtracestore.HoistedColumn, error) {
	type source struct {
		level     expression.Level
		key       string
		valueType pcommon.ValueType
	}
	sources := make(map[source]struct{}, len(attributes))
	takenNames := make(map[string]struct{})
	for _, match := range spansColumnPattern.FindAllStringSubmatch(sql.CreateSpansTable, -1) {
		takenNames[match[1]] = struct{}{}
	}
	columns := make([]chtracestore.HoistedColumn, 0, len(attributes))
	for i, attribute := range attributes {
		entryError := func(format string, args ...any) ([]chtracestore.HoistedColumn, error) {
			return nil, fmt.Errorf("hoisted_attributes[%d]: %s", i, fmt.Sprintf(format, args...))
		}
		if attribute.Key == "" {
			return entryError("key must not be empty")
		}
		level, ok := hoistedLevels[attribute.Level]
		if !ok {
			return entryError("level must be span or resource, not %q", attribute.Level)
		}
		valueType, ok := hoistedTypes[attribute.Type]
		if !ok {
			return entryError("type must be string, int, double or bool, not %q", attribute.Type)
		}
		if !hoistedColumnNamePattern.MatchString(attribute.Column) {
			return entryError("column %q must match [a-z][a-z0-9_]*", attribute.Column)
		}
		if _, exists := takenNames[attribute.Column]; exists {
			return entryError("column %q is already a column of the spans table or of another entry", attribute.Column)
		}
		takenNames[attribute.Column] = struct{}{}
		attributeSource := source{
			level:     level,
			key:       attribute.Key,
			valueType: valueType,
		}
		if _, exists := sources[attributeSource]; exists {
			return entryError("the %s attribute %q of type %s is listed twice", attribute.Level, attribute.Key, attribute.Type)
		}
		sources[attributeSource] = struct{}{}
		codec := defaultHoistedCodec
		if attribute.Codec != "" {
			codec, ok = zstdCodec(attribute.Codec)
			if !ok {
				return entryError("codec %q must be ZSTD(level) with a level from 1 through 22", attribute.Codec)
			}
		}
		columns = append(columns, chtracestore.HoistedColumn{
			Name:   attribute.Column,
			Level:  level,
			Key:    attribute.Key,
			Type:   valueType,
			Codec:  codec,
			Layout: layout,
		})
	}
	return columns, nil
}

// hoistedColumnState is how a table declares a hoisted column.
type hoistedColumnState struct {
	Type           string `ch:"type"`
	Kind           string `ch:"default_kind"`
	Expression     string `ch:"default_expression"`
	Codec          string `ch:"compression_codec"`
	SameExpression uint8  `ch:"same_expression"`
}

// declaration returns the stored type, kind, expression and codec in the order of a column
// declaration. An ordinary column has no kind and no expression.
func (s hoistedColumnState) declaration() string {
	parts := make([]string, 0, 4)
	for _, part := range []string{s.Type, s.Kind, s.Expression, s.Codec} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}

// checkHoistedColumns verifies that the spans table, and its storage table when spans is
// Distributed, declare every configured hoisted column with the type, expression and codec
// that the configuration renders. The tables
// may have been created by the operator or by an earlier configuration, and a search that read
// a column of another attribute would return wrong results without an error.
func checkHoistedColumns(ctx context.Context, conn driver.Conn, columns []chtracestore.HoistedColumn) error {
	if len(columns) == 0 {
		return nil
	}
	tables, err := hoistedColumnTables(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to look up the spans table: %w", err)
	}
	if len(tables) == 0 {
		return errors.New("hoisted attributes are configured, but the spans table does not exist; create the schema first")
	}
	for _, column := range columns {
		var addColumnStatements []string
		var mismatchErr error
		for _, table := range tables {
			state, found, err := readHoistedColumn(ctx, conn, table, column)
			if err != nil {
				return fmt.Errorf("failed to read the hoisted attribute column %q of the %s table: %w", column.Name, table, err)
			}
			if !found {
				addColumnStatements = append(addColumnStatements, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, column.Definition()))
				continue
			}
			if state.Type != column.NullableType() ||
				state.Kind != "MATERIALIZED" ||
				state.SameExpression == 0 ||
				state.Codec != "CODEC("+column.Codec+")" {
				mismatchErr = errors.Join(mismatchErr, fmt.Errorf("hoisted attribute column %q does not match its configuration: the %s table declares it as [%s], "+
					"and the configuration needs [%s]; a changed hoisted attribute needs a schema migration",
					column.Name, table, state.declaration(), column.Declaration()))
			}
		}
		var missingErr error
		if len(addColumnStatements) > 0 {
			missingErr = fmt.Errorf("hoisted attribute column %q is missing; add it with: %s", column.Name, strings.Join(addColumnStatements, "; "))
		}
		if err := errors.Join(missingErr, mismatchErr); err != nil {
			return err
		}
	}
	return nil
}

// readHoistedColumn reads the rows to the end, because the driver reports an error of the
// result stream only after the last row.
func readHoistedColumn(ctx context.Context, conn driver.Conn, table string, column chtracestore.HoistedColumn) (state hoistedColumnState, found bool, err error) {
	rows, err := conn.Query(ctx, sql.SelectHoistedColumn, column.SourceExpression(), table, column.Name)
	if err != nil {
		return state, false, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.ScanStruct(&state); err != nil {
			return state, false, err
		}
		found = true
	}
	return state, found, rows.Err()
}

// hoistedColumnTables returns the tables that must declare a hoisted column: spans, preceded by its
// storage table when spans is a Distributed table. A search fails on a shard when the
// Distributed table has the column and its storage table does not.
func hoistedColumnTables(ctx context.Context, conn driver.Conn) ([]string, error) {
	storageTable := spansTable + localTableSuffix
	rows, err := conn.Query(ctx, sql.SelectTableEngines, spansTable, storageTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	engines := map[string]string{}
	for rows.Next() {
		var name, engine string
		if err := rows.Scan(&name, &engine); err != nil {
			return nil, err
		}
		engines[name] = engine
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	engine, hasSpans := engines[spansTable]
	if !hasSpans {
		return nil, nil
	}
	if _, hasStorage := engines[storageTable]; hasStorage && engine == "Distributed" {
		return []string{storageTable, spansTable}, nil
	}
	return []string{spansTable}, nil
}
