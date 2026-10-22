// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// TestConvertTagField_NonFiniteTokens checks the flat tag-map read path.
// The three OTLP/JSON tokens come back as float64. Other strings, including
// the Go spellings "Inf" and "-Inf", stay strings.
func TestConvertTagField_NonFiniteTokens(t *testing.T) {
	reader := NewSpanReader(SpanReaderParams{Logger: zap.NewNop(), TagDotReplacement: "@"})
	span := &dbmodel.Span{
		Tag: map[string]any{
			"nan":        "NaN",
			"pinf":       "Infinity",
			"ninf":       "-Infinity",
			"goinf":      "Inf",
			"goninf":     "-Inf",
			"plus":       "+Infinity",
			"padded":     "NaN ",
			"word":       "kept",
			"ok":         json.Number("1.5"),
			"number.nan": json.Number("NaN"),
		},
		Process: dbmodel.Process{Tag: map[string]any{"proc@nan": "NaN"}},
	}
	reader.mergeAllNestedAndElevatedTagsOfSpan(span)

	assertFloatBack(t, findKeyValue(t, span.Tags, "nan"), math.NaN())
	assertFloatBack(t, findKeyValue(t, span.Tags, "pinf"), math.Inf(1))
	assertFloatBack(t, findKeyValue(t, span.Tags, "ninf"), math.Inf(-1))
	assertFloatBack(t, findKeyValue(t, span.Tags, "number.nan"), math.NaN())
	assertFloatBack(t, findKeyValue(t, span.Tags, "ok"), 1.5)
	assertFloatBack(t, findKeyValue(t, span.Process.Tags, "proc.nan"), math.NaN())

	for _, key := range []string{"goinf", "goninf", "plus", "padded", "word"} {
		kv := findKeyValue(t, span.Tags, key)
		assert.Equal(t, dbmodel.StringType, kv.Type, key)
		assert.IsType(t, "", kv.Value, key)
	}
	assert.Equal(t, "Inf", findKeyValue(t, span.Tags, "goinf").Value)
	assert.Equal(t, "-Inf", findKeyValue(t, span.Tags, "goninf").Value)
	assert.Equal(t, "+Infinity", findKeyValue(t, span.Tags, "plus").Value)
}
