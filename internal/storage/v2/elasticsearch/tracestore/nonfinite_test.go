// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func TestToDBModel_NonFiniteDoubles(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	rs.Resource().Attributes().PutDouble("proc.nan", math.NaN())
	rs.Resource().Attributes().PutDouble("proc.ok", 1.5)
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op")
	span.SetTraceID([16]byte{1})
	span.SetSpanID([8]byte{2})
	span.Attributes().PutDouble("nan", math.NaN())
	span.Attributes().PutDouble("pinf", math.Inf(1))
	span.Attributes().PutDouble("ninf", math.Inf(-1))
	span.Attributes().PutDouble("ok", 1.5)
	event := span.Events().AppendEmpty()
	event.SetName("boom")
	event.Attributes().PutDouble("ratio", math.Inf(-1))

	dbSpans := ToDBModel(td)
	require.Len(t, dbSpans, 1)
	got := dbSpans[0]

	assertStoredFloat(t, findDBTag(t, got.Tags, "nan"), math.NaN())
	assertStoredFloat(t, findDBTag(t, got.Tags, "pinf"), math.Inf(1))
	assertStoredFloat(t, findDBTag(t, got.Tags, "ninf"), math.Inf(-1))
	assertStoredFloat(t, findDBTag(t, got.Tags, "ok"), 1.5)
	assertStoredFloat(t, findDBTag(t, got.Process.Tags, "proc.nan"), math.NaN())
	assertStoredFloat(t, findDBTag(t, got.Process.Tags, "proc.ok"), 1.5)
	assertStoredFloat(t, findDBTag(t, got.Logs[0].Fields, "ratio"), math.Inf(-1))
}

// TestFromDBModel_NonFiniteFloatStrings is the read path for a document that
// already stores non-finite doubles as strings (the shape the writer emits)
// and for the float64 values the tag-map merge produces.
func TestFromDBModel_NonFiniteFloatStrings(t *testing.T) {
	t.Run("json strings", func(t *testing.T) {
		const doc = `{
			"traceID": "000000000000000000000000000000ab",
			"spanID": "00000000000000cd",
			"operationName": "op",
			"startTime": 10,
			"duration": 5,
			"process": {
				"serviceName": "svc",
				"tags": [
					{"key": "proc.nan", "type": "float64", "value": "NaN"},
					{"key": "proc.ok", "type": "float64", "value": 1.5}
				]
			},
			"tags": [
				{"key": "nan", "type": "float64", "value": "NaN"},
				{"key": "pinf", "type": "float64", "value": "Infinity"},
				{"key": "ninf", "type": "float64", "value": "-Infinity"},
				{"key": "ok", "type": "float64", "value": 1.5},
				{"key": "word", "type": "string", "value": "NaN"}
			],
			"logs": [
				{"timestamp": 11, "fields": [
					{"key": "event", "type": "string", "value": "boom"},
					{"key": "ratio", "type": "float64", "value": "-Infinity"}
				]}
			]
		}`
		dec := json.NewDecoder(strings.NewReader(doc))
		dec.UseNumber()
		var span dbmodel.Span
		require.NoError(t, dec.Decode(&span))

		td, err := FromDBModel([]dbmodel.Span{span})
		require.NoError(t, err)
		require.Equal(t, 1, td.ResourceSpans().Len())
		rs := td.ResourceSpans().At(0)
		requireDouble(t, rs.Resource().Attributes(), "proc.nan", math.NaN())
		requireDouble(t, rs.Resource().Attributes(), "proc.ok", 1.5)
		got := rs.ScopeSpans().At(0).Spans().At(0)
		requireDouble(t, got.Attributes(), "nan", math.NaN())
		requireDouble(t, got.Attributes(), "pinf", math.Inf(1))
		requireDouble(t, got.Attributes(), "ninf", math.Inf(-1))
		requireDouble(t, got.Attributes(), "ok", 1.5)
		word, ok := got.Attributes().Get("word")
		require.True(t, ok)
		assert.Equal(t, pcommon.ValueTypeStr, word.Type())
		assert.Equal(t, "NaN", word.Str())
		require.Equal(t, 1, got.Events().Len())
		assert.Equal(t, "boom", got.Events().At(0).Name())
		requireDouble(t, got.Events().At(0).Attributes(), "ratio", math.Inf(-1))
	})

	t.Run("float64 and json number", func(t *testing.T) {
		span := dbmodel.Span{
			TraceID:       "000000000000000000000000000000ab",
			SpanID:        "00000000000000cd",
			OperationName: "op",
			Process: dbmodel.Process{
				ServiceName: "svc",
				Tags: []dbmodel.KeyValue{
					{Key: "proc.nan", Type: dbmodel.Float64Type, Value: math.NaN()},
				},
			},
			Tags: []dbmodel.KeyValue{
				{Key: "nan", Type: dbmodel.Float64Type, Value: math.NaN()},
				{Key: "pinf", Type: dbmodel.Float64Type, Value: math.Inf(1)},
				{Key: "ninf", Type: dbmodel.Float64Type, Value: math.Inf(-1)},
				{Key: "num", Type: dbmodel.Float64Type, Value: json.Number("NaN")},
			},
			Logs: []dbmodel.Log{{
				Fields: []dbmodel.KeyValue{
					{Key: "ratio", Type: dbmodel.Float64Type, Value: math.Inf(1)},
				},
			}},
		}
		td, err := FromDBModel([]dbmodel.Span{span})
		require.NoError(t, err)
		rs := td.ResourceSpans().At(0)
		requireDouble(t, rs.Resource().Attributes(), "proc.nan", math.NaN())
		got := rs.ScopeSpans().At(0).Spans().At(0)
		requireDouble(t, got.Attributes(), "nan", math.NaN())
		requireDouble(t, got.Attributes(), "pinf", math.Inf(1))
		requireDouble(t, got.Attributes(), "ninf", math.Inf(-1))
		requireDouble(t, got.Attributes(), "num", math.NaN())
		requireDouble(t, got.Events().At(0).Attributes(), "ratio", math.Inf(1))
	})
}

func findDBTag(t *testing.T, kvs []dbmodel.KeyValue, key string) dbmodel.KeyValue {
	t.Helper()
	for _, kv := range kvs {
		if kv.Key == key {
			return kv
		}
	}
	t.Fatalf("missing tag %q", key)
	return dbmodel.KeyValue{}
}

func assertStoredFloat(t *testing.T, kv dbmodel.KeyValue, want float64) {
	t.Helper()
	assert.Equal(t, dbmodel.Float64Type, kv.Type)
	got, ok := kv.Value.(float64)
	require.True(t, ok, "value %T", kv.Value)
	switch {
	case math.IsNaN(want):
		assert.True(t, math.IsNaN(got))
	case math.IsInf(want, 1):
		assert.True(t, math.IsInf(got, 1))
	case math.IsInf(want, -1):
		assert.True(t, math.IsInf(got, -1))
	default:
		assert.InDelta(t, want, got, 0)
	}
}

func requireDouble(t *testing.T, attrs pcommon.Map, key string, want float64) {
	t.Helper()
	v, ok := attrs.Get(key)
	require.True(t, ok, "missing %s", key)
	require.Equal(t, pcommon.ValueTypeDouble, v.Type(), key)
	switch {
	case math.IsNaN(want):
		assert.True(t, math.IsNaN(v.Double()), key)
	case math.IsInf(want, 1):
		assert.True(t, math.IsInf(v.Double(), 1), key)
	case math.IsInf(want, -1):
		assert.True(t, math.IsInf(v.Double(), -1), key)
	default:
		assert.InDelta(t, want, v.Double(), 0, key)
	}
}
