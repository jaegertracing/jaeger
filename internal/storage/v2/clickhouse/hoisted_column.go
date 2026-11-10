// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"fmt"
	"regexp"
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
)

const defaultHoistedCodec = "ZSTD(1)"

var (
	hoistedColumnNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
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

// hoistedColumn is a validated HoistedAttribute.
type hoistedColumn struct {
	attribute chtracestore.HoistedAttribute
	codec     string
}

// definition returns the column definition as it appears in CREATE TABLE. The name is quoted
// because a name such as index is a keyword at that position.
func (c hoistedColumn) definition() string {
	return "`" + c.attribute.Column + "` " + fmt.Sprintf("%s MATERIALIZED %s CODEC(%s)",
		c.attribute.ColumnType(), c.attribute.SourceExpression(), c.codec)
}

// zstdCodec returns codec as ClickHouse reports it, and false unless codec is ZSTD(level) with
// a level that ClickHouse accepts. No other codec is accepted, so the value can be rendered
// into DDL without being arbitrary SQL. The level is rendered from its number because
// ClickHouse stores ZSTD(01) as ZSTD(1), and the startup check compares the stored text.
func zstdCodec(codec string) (string, bool) {
	m := hoistedCodecPattern.FindStringSubmatch(codec)
	if m == nil {
		return "", false
	}
	// The pattern admits one or two digits, so the conversion cannot fail.
	level, _ := strconv.Atoi(m[1])
	if level < 1 || level > 22 {
		return "", false
	}
	return fmt.Sprintf("ZSTD(%d)", level), true
}

// hoistedColumns validates HoistedAttributes and returns the columns they describe, in the
// configured order. Validation and conversion share one function so that no column can be
// rendered from an entry that was not checked.
func (cfg *Configuration) hoistedColumns() ([]hoistedColumn, error) {
	type source struct {
		level expression.Level
		key   string
		typ   pcommon.ValueType
	}
	sources := make(map[source]struct{}, len(cfg.HoistedAttributes))
	names := make(map[string]struct{})
	for _, match := range spansColumnPattern.FindAllStringSubmatch(sql.CreateSpansTable, -1) {
		names[match[1]] = struct{}{}
	}
	columns := make([]hoistedColumn, 0, len(cfg.HoistedAttributes))
	for i, h := range cfg.HoistedAttributes {
		fail := func(format string, args ...any) ([]hoistedColumn, error) {
			return nil, fmt.Errorf("hoisted_attributes[%d]: %s", i, fmt.Sprintf(format, args...))
		}
		if h.Key == "" {
			return fail("key must not be empty")
		}
		level, ok := hoistedLevels[h.Level]
		if !ok {
			return fail("level must be span or resource, not %q", h.Level)
		}
		typ, ok := hoistedTypes[h.Type]
		if !ok {
			return fail("type must be string, int, double or bool, not %q", h.Type)
		}
		if !hoistedColumnNamePattern.MatchString(h.Column) {
			return fail("column %q must match [a-z_][a-z0-9_]*", h.Column)
		}
		if _, taken := names[h.Column]; taken {
			return fail("column %q is already a column of the spans table or of another entry", h.Column)
		}
		names[h.Column] = struct{}{}
		src := source{level: level, key: h.Key, typ: typ}
		if _, dup := sources[src]; dup {
			return fail("the %s attribute %q of type %s is listed twice", h.Level, h.Key, h.Type)
		}
		sources[src] = struct{}{}
		codec := defaultHoistedCodec
		if h.Codec != "" {
			codec, ok = zstdCodec(h.Codec)
		}
		if !ok {
			return fail("codec %q must be ZSTD(level) with a level from 1 through 22", h.Codec)
		}
		columns = append(columns, hoistedColumn{
			attribute: chtracestore.HoistedAttribute{
				Level:  level,
				Key:    h.Key,
				Type:   typ,
				Column: h.Column,
			},
			codec: codec,
		})
	}
	return columns, nil
}
