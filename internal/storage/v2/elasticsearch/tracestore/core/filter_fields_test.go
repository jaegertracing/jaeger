// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func TestIndexedBuiltInFilters(t *testing.T) {
	tests := []struct {
		level expression.Level
		name  string
		value string
	}{
		{expression.LevelSpan, "traceID", "0123456789abcdef0123456789abcdef"},
		{expression.LevelSpan, "spanID", "0123456789abcdef"},
		{expression.LevelScope, "name", "instrumentation.cart"},
		{expression.LevelScope, "version", "1.2.3"},
		{expression.LevelLink, "traceID", "0123456789abcdef0123456789abcdef"},
		{expression.LevelLink, "spanID", "0123456789abcdef"},
		{expression.LevelSpan, "startTime", "2026-09-01T12:34:56.123456789+05:00"},
		{expression.LevelEvent, "time", "2026-09-01T12:34:56.123456789+05:00"},
	}
	for _, test := range tests {
		t.Run(string(test.level)+"."+test.name, func(t *testing.T) {
			ref := &expression.FieldRef{Level: test.level, Name: test.name}
			queries := make(map[string]any)
			withSpanReader(t, func(r *spanReaderTest) {
				for _, op := range []expression.Operator{
					expression.OpEq, expression.OpNe, expression.OpIn, expression.OpNotIn,
					expression.OpExists, expression.OpGt, expression.OpGte, expression.OpLt, expression.OpLte,
				} {
					t.Run(string(op), func(t *testing.T) {
						predicate := call(op, ref, scalar(test.value))
						switch op {
						case expression.OpExists:
							predicate.Args = predicate.Args[:1]
						case expression.OpIn, expression.OpNotIn:
							predicate.Args[1] = &expression.List{Values: []string{test.value}}
						default:
						}
						require.NoError(t, FilterCapabilities().EnsureSupported(predicate))
						query, err := r.reader.buildFilterQuery(predicate)
						require.NoError(t, err)
						source, err := query.Source()
						require.NoError(t, err)
						queries[string(op)] = source
					})
				}
			})
			data, err := json.MarshalIndent(queries, "", "  ")
			require.NoError(t, err)
			snapshottest.Assert(t, "testdata/builtin-filter/"+string(test.level)+"_"+test.name, string(data))
		})
	}
}

func TestIndexedBuiltInFilterRefusals(t *testing.T) {
	fields := []*expression.FieldRef{
		{Level: expression.LevelSpan, Name: "traceID"},
		{Level: expression.LevelSpan, Name: "spanID"},
		{Level: expression.LevelScope, Name: "name"},
		{Level: expression.LevelScope, Name: "version"},
		{Level: expression.LevelLink, Name: "traceID"},
		{Level: expression.LevelLink, Name: "spanID"},
		{Level: expression.LevelSpan, Name: "startTime"},
		{Level: expression.LevelEvent, Name: "time"},
	}
	withSpanReader(t, func(r *spanReaderTest) {
		for _, ref := range fields {
			t.Run(string(ref.Level)+"."+ref.Name, func(t *testing.T) {
				for _, predicate := range []*expression.Call{
					call(expression.OpEq, ref, &expression.BoolValue{Value: true}),
					call(expression.OpIn, ref, &expression.List{Type: expression.ValueTypeBool, Values: []string{"true"}}),
					call(expression.Operator("unknown"), ref, scalar("value")),
				} {
					query, err := r.reader.buildFilterQuery(predicate)
					require.Error(t, err)
					assert.Nil(t, query)
				}
				if ref.Level == expression.LevelLink {
					query, err := r.reader.buildFilterQuery(call(expression.OpRegex, ref, scalar("[a-f]+")))
					require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
					assert.Contains(t, err.Error(), "exact link identity")
					assert.Nil(t, query)
				}
				if ref.Level == expression.LevelScope {
					return
				}
				for _, op := range []expression.Operator{expression.OpEq, expression.OpNe, expression.OpIn, expression.OpNotIn} {
					predicate := call(op, ref, scalar("malformed"))
					if op == expression.OpIn || op == expression.OpNotIn {
						predicate.Args[1] = &expression.List{Values: []string{"malformed"}}
					}
					query, err := r.reader.buildFilterQuery(predicate)
					require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
					assert.Nil(t, query)
				}
			})
		}
		for _, ref := range []*expression.FieldRef{
			{Level: expression.LevelSpan, Name: "kind"},
			{Level: expression.LevelSpan, Name: "status"},
			{Level: expression.LevelSpan, Name: "endTime"},
			{Level: expression.LevelScope, Name: "schemaURL"},
			{Level: expression.LevelLink, Name: "traceState"},
		} {
			query, err := r.reader.buildFilterQuery(call(expression.OpExists, ref))
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Contains(t, err.Error(), "built-in field")
			assert.Nil(t, query)
		}
		for _, level := range []expression.Level{expression.LevelScope, expression.LevelLink} {
			query, err := r.reader.buildFilterQuery(call(expression.OpExists, &expression.AttributeRef{Level: level, Key: "custom"}))
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Contains(t, err.Error(), `does not index attribute "custom"`)
			assert.Nil(t, query)
			query, err = r.reader.buildFilterQuery(call(expression.OpSome, &expression.NestedRef{Level: level}, call(expression.OpExists, fields[0])))
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Nil(t, query)
		}
	})
}

func TestTimestampFilterPrecision(t *testing.T) {
	instant := time.Date(2026, 9, 1, 12, 34, 56, 123456789, time.FixedZone("offset", 5*60*60))
	for _, test := range []struct {
		field string
		want  int64
	}{
		{startTimeMillisField, instant.UnixMilli()},
		{"logs.timestamp", instant.UnixMicro()},
	} {
		t.Run(test.field, func(t *testing.T) {
			query, err := timestampComparison(test.field, expression.OpEq, &expression.TimestampValue{Value: instant})
			require.NoError(t, err)
			source, err := query.Source()
			require.NoError(t, err)
			got, err := json.Marshal(source)
			require.NoError(t, err)
			want, err := json.Marshal(map[string]any{"term": map[string]any{test.field: test.want}})
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(got))
			utc, err := timestampComparison(test.field, expression.OpEq, scalar(instant.UTC().Format(time.RFC3339Nano)))
			require.NoError(t, err)
			utcSource, err := utc.Source()
			require.NoError(t, err)
			assert.Equal(t, source, utcSource)
		})
	}
}

func TestTimestampFilterInvalidValues(t *testing.T) {
	for _, value := range []expression.Expression{
		(*expression.AnyValue)(nil), (*expression.TimestampValue)(nil),
		&expression.StringValue{Value: "2026-09-01T00:00:00Z"},
		&expression.TimestampValue{Value: time.Time{}},
		&expression.TimestampValue{Value: time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		query, err := timestampComparison(startTimeMillisField, expression.OpEq, value)
		require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
		assert.Nil(t, query)
	}
	query, err := timestampComparison(startTimeMillisField, expression.OpRegex, scalar(".*"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	assert.Contains(t, err.Error(), "on a timestamp")
	assert.Nil(t, query)
}

func TestIndexedBuiltInPatternsAndNestedComposition(t *testing.T) {
	field := func(level expression.Level, name string) *expression.FieldRef {
		return &expression.FieldRef{Level: level, Name: name}
	}
	linkTrace := field(expression.LevelLink, "traceID")
	linkSpan := field(expression.LevelLink, "spanID")
	eventTime := field(expression.LevelEvent, "time")
	queries := make(map[string]any)
	withSpanReader(t, func(r *spanReaderTest) {
		r.reader.dotReplacer = dbmodel.NewDotReplacer("_")
		tests := map[string]*expression.Call{
			"scope name custom dot replacement": call(expression.OpEq, field(expression.LevelScope, "name"), scalar("cart")),
			"scope version ordered as text":     call(expression.OpGt, field(expression.LevelScope, "version"), scalar("1.2")),
			"link ID predicates are independently existential": call(expression.OpAnd,
				call(expression.OpEq, linkTrace, scalar("0123456789abcdef0123456789abcdef")),
				call(expression.OpEq, linkSpan, scalar("fedcba9876543210"))),
			"event time predicates are independently existential": call(expression.OpAnd,
				call(expression.OpGt, eventTime, scalar("2026-09-01T00:00:00Z")),
				call(expression.OpLt, eventTime, scalar("2026-09-02T00:00:00Z"))),
			"multiple link IDs excluded outside the nested query": call(expression.OpNotIn, linkSpan,
				&expression.List{Values: []string{"0123456789abcdef", "fedcba9876543210"}}),
			"uppercase ID literal": call(expression.OpEq, linkSpan, scalar("0123456789ABCDEF")),
		}
		for _, ref := range []*expression.FieldRef{
			field(expression.LevelSpan, "traceID"), field(expression.LevelSpan, "spanID"),
			field(expression.LevelScope, "name"), field(expression.LevelScope, "version"), linkTrace, linkSpan,
		} {
			if ref.Level != expression.LevelLink {
				tests[string(ref.Level)+"."+ref.Name+" regex"] = call(expression.OpRegex, ref, scalar("[a-f]+"))
			}
			query, err := r.reader.buildFilterQuery(call(expression.OpRegex, ref, scalar(`\d`)))
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Nil(t, query)
		}
		for name, predicate := range tests {
			t.Run(name, func(t *testing.T) {
				query, err := r.reader.buildFilterQuery(predicate)
				require.NoError(t, err)
				source, err := query.Source()
				require.NoError(t, err)
				queries[name] = source
			})
		}
	})
	data, err := json.MarshalIndent(queries, "", "  ")
	require.NoError(t, err)
	snapshottest.Assert(t, "testdata/builtin-filter/composition", string(data))
}

func TestIndexedTextMatchUnknownOperator(t *testing.T) {
	match, err := indexedTextMatch(expression.Operator("unknown"), reference{level: expression.LevelScope, name: "name"}, "cart")
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	assert.Nil(t, match)
}
