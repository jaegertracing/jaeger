// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// TestHoistedColumn_Definition snapshots the column definitions that CREATE TABLE gets, one
// per stored type and level, plus a key that needs escaping and a name that is a keyword.
func TestHoistedColumn_Definition(t *testing.T) {
	columns := []HoistedColumn{
		{
			Name:  "span_str",
			Level: expression.LevelSpan,
			Key:   "gen_ai.request.model",
			Type:  pcommon.ValueTypeStr,
			Codec: "ZSTD(1)",
		},
		{
			Name:  "span_int",
			Level: expression.LevelSpan,
			Key:   "gen_ai.usage.input_tokens",
			Type:  pcommon.ValueTypeInt,
			Codec: "ZSTD(1)",
		},
		{
			Name:  "span_double",
			Level: expression.LevelSpan,
			Key:   "ratio",
			Type:  pcommon.ValueTypeDouble,
			Codec: "ZSTD(1)",
		},
		{
			Name:  "span_bool",
			Level: expression.LevelSpan,
			Key:   "cached",
			Type:  pcommon.ValueTypeBool,
			Codec: "ZSTD(1)",
		},
		{
			Name:  "resource_int",
			Level: expression.LevelResource,
			Key:   "shard",
			Type:  pcommon.ValueTypeInt,
			Codec: "ZSTD(3)",
		},
		{
			Name:  "index",
			Level: expression.LevelResource,
			Key:   `it's a \key`,
			Type:  pcommon.ValueTypeStr,
			Codec: "ZSTD(1)",
		},
	}
	definitions := make([]string, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, column.Definition())
	}
	verifyQuerySnapshot(t, strings.Join(definitions, ",\n"))
}
