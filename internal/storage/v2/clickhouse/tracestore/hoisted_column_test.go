// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
)

// TestHoistedColumn_Definition snapshots the column definitions that CREATE TABLE gets, one
// per stored type and level, plus a key that needs escaping, a name that is a keyword, and two
// columns of a table in the map layout.
func TestHoistedColumn_Definition(t *testing.T) {
	columns := []HoistedColumn{
		{
			Name:   "span_str",
			Level:  expression.LevelSpan,
			Key:    "gen_ai.request.model",
			Type:   pcommon.ValueTypeStr,
			Codec:  "ZSTD(1)",
			Layout: NestedLayout{},
		},
		{
			Name:   "span_int",
			Level:  expression.LevelSpan,
			Key:    "gen_ai.usage.input_tokens",
			Type:   pcommon.ValueTypeInt,
			Codec:  "ZSTD(1)",
			Layout: NestedLayout{},
		},
		{
			Name:   "span_double",
			Level:  expression.LevelSpan,
			Key:    "ratio",
			Type:   pcommon.ValueTypeDouble,
			Codec:  "ZSTD(1)",
			Layout: NestedLayout{},
		},
		{
			Name:   "span_bool",
			Level:  expression.LevelSpan,
			Key:    "cached",
			Type:   pcommon.ValueTypeBool,
			Codec:  "ZSTD(1)",
			Layout: NestedLayout{},
		},
		{
			Name:   "resource_int",
			Level:  expression.LevelResource,
			Key:    "shard",
			Type:   pcommon.ValueTypeInt,
			Codec:  "ZSTD(3)",
			Layout: NestedLayout{},
		},
		{
			Name:   "index",
			Level:  expression.LevelResource,
			Key:    `it's a \key`,
			Type:   pcommon.ValueTypeStr,
			Codec:  "ZSTD(1)",
			Layout: NestedLayout{},
		},
		{
			Name:   "map_span_int",
			Level:  expression.LevelSpan,
			Key:    "gen_ai.usage.input_tokens",
			Type:   pcommon.ValueTypeInt,
			Codec:  "ZSTD(1)",
			Layout: MapLayout{},
		},
		{
			Name:   "map_resource_str",
			Level:  expression.LevelResource,
			Key:    `it's a \key`,
			Type:   pcommon.ValueTypeStr,
			Codec:  "ZSTD(1)",
			Layout: MapLayout{},
		},
	}
	definitions := make([]string, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, column.Definition())
	}
	clickhousetest.VerifyQuerySnapshot(t, strings.Join(definitions, ",\n"))
}
