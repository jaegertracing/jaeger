// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

// jsonFloatToken is the encoding/json literal for a float64 tag value.
// Non-finite values must be the OTLP/JSON strings; 1.5 stays a JSON number.
func jsonFloatToken(v float64) string {
	switch {
	case math.IsNaN(v):
		return `"NaN"`
	case math.IsInf(v, 1):
		return `"Infinity"`
	case math.IsInf(v, -1):
		return `"-Infinity"`
	default:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
}

func floatTagValue(v float64) any {
	token := jsonFloatToken(v)
	if strings.HasPrefix(token, `"`) {
		return strings.Trim(token, `"`)
	}
	return v
}

// spanCarryingFloat puts v into every float-bearing spot on a span document:
// span tags, process tags, log fields, and both flat tag maps. 1.5 and a
// string tag ride along as controls.
func spanCarryingFloat(v float64) *dbmodel.Span {
	return &dbmodel.Span{
		TraceID:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SpanID:        "bbbbbbbbbbbbbbbb",
		OperationName: "op",
		StartTime:     1,
		Tags: []dbmodel.KeyValue{
			{Key: "span", Type: dbmodel.Float64Type, Value: v},
			{Key: "ok", Type: dbmodel.Float64Type, Value: 1.5},
			{Key: "label", Type: dbmodel.StringType, Value: "kept"},
		},
		Tag: map[string]any{
			"flat":      v,
			"flatOk":    1.5,
			"flatLabel": "kept",
		},
		Logs: []dbmodel.Log{{
			Timestamp: 2,
			Fields: []dbmodel.KeyValue{
				{Key: "log", Type: dbmodel.Float64Type, Value: v},
				{Key: "note", Type: dbmodel.StringType, Value: "x"},
			},
		}},
		Process: dbmodel.Process{
			ServiceName: "svc",
			Tags: []dbmodel.KeyValue{
				{Key: "proc", Type: dbmodel.Float64Type, Value: v},
				{Key: "procOk", Type: dbmodel.Float64Type, Value: 1.5},
			},
			Tag: map[string]any{
				"procFlat":   v,
				"procFlatOk": 1.5,
			},
		},
	}
}

func newFloatSpanWriter(allTagsAsFields bool, batch *fakeBatchWriter) (*SpanWriter, *testutils.Buffer, *metricstest.Factory) {
	logger, buf := testutils.NewLogger()
	mf := metricstest.NewFactory(0)
	if batch == nil {
		batch = &fakeBatchWriter{}
	}
	w := NewSpanWriter(SpanWriterParams{
		BatchWriter:       batch,
		Logger:            logger,
		MetricsFactory:    mf,
		ServiceOperations: NewServiceOperationStorage(nil, logger, 0),
		AllTagsAsFields:   allTagsAsFields,
		TagDotReplacement: "@",
		SpanRotation:      indices.NewAliasedRotation("jaeger-span-write", "jaeger-span-read"),
		ServiceRotation:   indices.NewAliasedRotation("jaeger-service-write", "jaeger-service-read"),
	})
	return w, buf, mf
}

func spanRawBodies(items []esclient.BulkItem) []string {
	var out []string
	for _, item := range items {
		raw, ok := item.Body.(json.RawMessage)
		if ok {
			out = append(out, string(raw))
		}
	}
	return out
}

func assertFloatJSON(t *testing.T, raw, key, token string) {
	t.Helper()
	assert.Contains(t, raw, `"key":"`+key+`","type":"float64","value":`+token)
}

// TestBuildSpanItem_NonFiniteFloatTags is the writer contract for double tags.
// NaN, +Inf and -Inf are stored as the strings "NaN", "Infinity" and "-Infinity"
// with type left as float64. The finite control 1.5 stays a JSON number.
// The same value is checked on span tags, process tags, log fields, and both
// flat tag maps.
func TestBuildSpanItem_NonFiniteFloatTags(t *testing.T) {
	cases := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "+Inf", value: math.Inf(1)},
		{name: "-Inf", value: math.Inf(-1)},
		{name: "finite", value: 1.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer, _, _ := newFloatSpanWriter(false, nil)
			span := spanCarryingFloat(tc.value)
			token := jsonFloatToken(tc.value)

			item, err := writer.buildSpanItem("jaeger-span-write", span)
			require.NoError(t, err)
			raw, ok := item.Body.(json.RawMessage)
			require.True(t, ok)
			body := string(raw)

			assertFloatJSON(t, body, "span", token)
			assertFloatJSON(t, body, "proc", token)
			assertFloatJSON(t, body, "log", token)
			assertFloatJSON(t, body, "ok", "1.5")
			assertFloatJSON(t, body, "procOk", "1.5")
			assert.Contains(t, body, `"key":"label","type":"string","value":"kept"`)
			assert.Contains(t, body, `"flat":`+token)
			assert.Contains(t, body, `"flatOk":1.5`)
			assert.Contains(t, body, `"flatLabel":"kept"`)
			assert.Contains(t, body, `"procFlat":`+token)
			assert.Contains(t, body, `"procFlatOk":1.5`)

			want := floatTagValue(tc.value)
			assert.Equal(t, dbmodel.Float64Type, span.Tags[0].Type)
			assert.Equal(t, want, span.Tags[0].Value)
			assert.Equal(t, dbmodel.Float64Type, span.Process.Tags[0].Type)
			assert.Equal(t, want, span.Process.Tags[0].Value)
			assert.Equal(t, dbmodel.Float64Type, span.Logs[0].Fields[0].Type)
			assert.Equal(t, want, span.Logs[0].Fields[0].Value)
			assert.Equal(t, want, span.Tag["flat"])
			assert.Equal(t, want, span.Process.Tag["procFlat"])
			okVal, ok := span.Tags[1].Value.(float64)
			require.True(t, ok)
			assert.InDelta(t, 1.5, okVal, 0)
			assert.Equal(t, "kept", span.Tags[2].Value)

			again, err := writer.buildSpanItem("jaeger-span-write", span)
			require.NoError(t, err)
			assert.Equal(t, item.ID, again.ID)
			assert.Equal(t, body, string(again.Body.(json.RawMessage)))
		})
	}
}

// TestWriteSpans_NonFiniteFloatNotDropped pins that a non-finite double does not
// drop the span or log an encode error, whether the value stays nested or is
// elevated into the flat tag map.
func TestWriteSpans_NonFiniteFloatNotDropped(t *testing.T) {
	cases := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "+Inf", value: math.Inf(1)},
		{name: "-Inf", value: math.Inf(-1)},
		{name: "finite", value: 1.5},
	}
	for _, allTags := range []bool{false, true} {
		t.Run("allTagsAsFields="+strconv.FormatBool(allTags), func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					batch := &fakeBatchWriter{}
					writer, logs, mf := newFloatSpanWriter(allTags, batch)
					span := spanCarryingFloat(tc.value)
					// WriteSpans rebuilds the flat maps from the nested tags.
					span.Tag = nil
					span.Process.Tag = nil
					token := jsonFloatToken(tc.value)

					require.NoError(t, writer.WriteSpans(context.Background(), []dbmodel.Span{*span}))
					logged := logs.String()
					assert.NotContains(t, logged, "failed to encode span document")
					assert.NotContains(t, logged, "unsupported value")
					mf.AssertCounterMetrics(t,
						metricstest.ExpectedMetric{Name: "spans.attempts", Value: 1},
						metricstest.ExpectedMetric{Name: "spans.errors", Value: 0},
					)

					bodies := spanRawBodies(batch.items)
					require.Len(t, bodies, 1, "logged: %s", logged)
					raw := bodies[0]
					// Log fields are never elevated, so the float stays a typed KeyValue.
					assertFloatJSON(t, raw, "log", token)
					if allTags {
						assert.Contains(t, raw, `"span":`+token)
						assert.Contains(t, raw, `"proc":`+token)
						assert.Contains(t, raw, `"ok":1.5`)
						assert.Contains(t, raw, `"label":"kept"`)
						assert.NotContains(t, raw, `"key":"span"`)
						assert.NotContains(t, raw, `"key":"ok"`)
					} else {
						assertFloatJSON(t, raw, "span", token)
						assertFloatJSON(t, raw, "proc", token)
						assertFloatJSON(t, raw, "ok", "1.5")
						assert.Contains(t, raw, `"key":"label","type":"string","value":"kept"`)
					}
				})
			}
		})
	}
}

// TestNonFiniteFloat_ReaderRoundTrip writes a span the way the collector does
// and reads it back through the ES JSON decoder plus tag-map merge.
func TestNonFiniteFloat_ReaderRoundTrip(t *testing.T) {
	reader := NewSpanReader(SpanReaderParams{Logger: zap.NewNop(), TagDotReplacement: "@"})

	t.Run("nested", func(t *testing.T) {
		batch := &fakeBatchWriter{}
		writer, _, _ := newFloatSpanWriter(false, batch)
		span := spanCarryingFloat(math.NaN())
		span.Tags = append(span.Tags,
			dbmodel.KeyValue{Key: "pinf", Type: dbmodel.Float64Type, Value: math.Inf(1)},
			dbmodel.KeyValue{Key: "ninf", Type: dbmodel.Float64Type, Value: math.Inf(-1)},
		)
		span.Tag = nil
		span.Process.Tag = nil
		require.NoError(t, writer.WriteSpans(context.Background(), []dbmodel.Span{*span}))
		bodies := spanRawBodies(batch.items)
		require.Len(t, bodies, 1)

		got, err := reader.unmarshalJSONSpan(esclient.SearchHit{Source: json.RawMessage(bodies[0])})
		require.NoError(t, err)
		reader.mergeAllNestedAndElevatedTagsOfSpan(&got)

		nan := findKeyValue(t, got.Tags, "span")
		assert.Equal(t, dbmodel.Float64Type, nan.Type)
		assert.Equal(t, "NaN", nan.Value)
		assert.Equal(t, "Infinity", findKeyValue(t, got.Tags, "pinf").Value)
		assert.Equal(t, "-Infinity", findKeyValue(t, got.Tags, "ninf").Value)
		assert.InDelta(t, 1.5, floatFromNumber(t, findKeyValue(t, got.Tags, "ok").Value), 0)
		assert.Equal(t, "kept", findKeyValue(t, got.Tags, "label").Value)
		assert.Equal(t, "NaN", findKeyValue(t, got.Process.Tags, "proc").Value)
		assert.Equal(t, "NaN", findKeyValue(t, got.Logs[0].Fields, "log").Value)
		assert.Equal(t, dbmodel.Float64Type, findKeyValue(t, got.Logs[0].Fields, "log").Type)
	})

	t.Run("tags as fields", func(t *testing.T) {
		batch := &fakeBatchWriter{}
		writer, _, _ := newFloatSpanWriter(true, batch)
		span := &dbmodel.Span{
			TraceID:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SpanID:        "bbbbbbbbbbbbbbbb",
			OperationName: "op",
			StartTime:     1,
			Process: dbmodel.Process{
				ServiceName: "svc",
				Tags: []dbmodel.KeyValue{
					{Key: "proc.nan", Type: dbmodel.Float64Type, Value: math.NaN()},
				},
			},
			Tags: []dbmodel.KeyValue{
				{Key: "a.b", Type: dbmodel.Float64Type, Value: math.NaN()},
				{Key: "pinf", Type: dbmodel.Float64Type, Value: math.Inf(1)},
				{Key: "ninf", Type: dbmodel.Float64Type, Value: math.Inf(-1)},
				{Key: "ok", Type: dbmodel.Float64Type, Value: 1.5},
				{Key: "label", Type: dbmodel.StringType, Value: "kept"},
				{Key: "word", Type: dbmodel.StringType, Value: "NaN"},
			},
			Logs: []dbmodel.Log{{
				Fields: []dbmodel.KeyValue{
					{Key: "ratio", Type: dbmodel.Float64Type, Value: math.Inf(-1)},
				},
			}},
		}
		require.NoError(t, writer.WriteSpans(context.Background(), []dbmodel.Span{*span}))
		bodies := spanRawBodies(batch.items)
		require.Len(t, bodies, 1)
		assert.Contains(t, bodies[0], `"a@b":"NaN"`)
		assert.Contains(t, bodies[0], `"pinf":"Infinity"`)
		assert.Contains(t, bodies[0], `"ninf":"-Infinity"`)
		assert.Contains(t, bodies[0], `"ok":1.5`)
		assert.Contains(t, bodies[0], `"label":"kept"`)
		assert.Contains(t, bodies[0], `"word":"NaN"`)
		assert.Contains(t, bodies[0], `"proc@nan":"NaN"`)
		assert.Contains(t, bodies[0], `"key":"ratio","type":"float64","value":"-Infinity"`)

		got, err := reader.unmarshalJSONSpan(esclient.SearchHit{Source: json.RawMessage(bodies[0])})
		require.NoError(t, err)
		reader.mergeAllNestedAndElevatedTagsOfSpan(&got)

		assertFloatBack(t, findKeyValue(t, got.Tags, "a.b"), math.NaN())
		assertFloatBack(t, findKeyValue(t, got.Tags, "pinf"), math.Inf(1))
		assertFloatBack(t, findKeyValue(t, got.Tags, "ninf"), math.Inf(-1))
		assertFloatBack(t, findKeyValue(t, got.Tags, "ok"), 1.5)
		label := findKeyValue(t, got.Tags, "label")
		assert.Equal(t, dbmodel.StringType, label.Type)
		assert.Equal(t, "kept", label.Value)
		// A string attribute whose value is the token is indistinguishable from
		// a non-finite float once both live in the untyped tag object.
		word := findKeyValue(t, got.Tags, "word")
		assert.Equal(t, dbmodel.Float64Type, word.Type)
		wordF, ok := word.Value.(float64)
		require.True(t, ok, "word value %T", word.Value)
		assert.True(t, math.IsNaN(wordF))
		assertFloatBack(t, findKeyValue(t, got.Process.Tags, "proc.nan"), math.NaN())
		ratio := findKeyValue(t, got.Logs[0].Fields, "ratio")
		assert.Equal(t, dbmodel.Float64Type, ratio.Type)
		assert.Equal(t, "-Infinity", ratio.Value)
	})
}

func findKeyValue(t *testing.T, kvs []dbmodel.KeyValue, key string) dbmodel.KeyValue {
	t.Helper()
	for _, kv := range kvs {
		if kv.Key == key {
			return kv
		}
	}
	t.Fatalf("missing key %q in %+v", key, kvs)
	return dbmodel.KeyValue{}
}

func assertFloatBack(t *testing.T, kv dbmodel.KeyValue, want float64) {
	t.Helper()
	assert.Equal(t, dbmodel.Float64Type, kv.Type)
	got := floatFromNumber(t, kv.Value)
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

func floatFromNumber(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, err := n.Float64()
		require.NoError(t, err)
		return f
	default:
		t.Fatalf("value %T (%v) is not a float", v, v)
		return 0
	}
}
