// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

func call(op expression.Operator, args ...expression.Expression) *expression.Call {
	return &expression.Call{Op: op, Args: args}
}

func fieldRef(level expression.Level, name string) *expression.FieldRef {
	return &expression.FieldRef{Level: level, Name: name}
}

func attrRef(level expression.Level, key string) *expression.AttributeRef {
	return &expression.AttributeRef{Level: level, Key: key}
}

func str(v string) *expression.StringValue { return &expression.StringValue{Value: v} }

func TestFilterCapabilities(t *testing.T) {
	caps := FilterCapabilities()
	assert.ElementsMatch(t, []expression.Level{
		expression.LevelSpan, expression.LevelResource, expression.LevelScope,
		expression.LevelEvent, expression.LevelLink,
	}, caps.Levels)
	assert.ElementsMatch(t, []expression.Operator{
		expression.OpAnd, expression.OpOr, expression.OpNot, expression.OpEq,
	}, caps.Operators)
}

func newTestReader() *Reader {
	return NewReader(&clickhousetest.Driver{}, testReaderConfig)
}

func TestBuildFilterCondition_And(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpAnd,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("checkout")),
		call(expression.OpEq, fieldRef(expression.LevelResource, expression.ResourceFieldService), str("cart")),
	)
	args, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.NoError(t, err)
	verifyQuerySnapshot(t, q.String())
	assert.Equal(t, []any{"checkout", "cart"}, args)
}

func TestBuildFilterCondition_Or(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpOr,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("checkout")),
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("cart")),
	)
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.NoError(t, err)
	verifyQuerySnapshot(t, q.String())
}

func TestBuildFilterCondition_Not(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpNot,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("checkout")),
	)
	args, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.NoError(t, err)
	verifyQuerySnapshot(t, q.String())
	assert.Equal(t, []any{"checkout"}, args)
}

func TestBuildFilterCondition_NestedBoolean(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpAnd,
		call(expression.OpOr,
			call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("a")),
			call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("b")),
		),
		call(expression.OpNot,
			call(expression.OpEq, fieldRef(expression.LevelResource, expression.ResourceFieldService), str("noisy")),
		),
	)
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.NoError(t, err)
	verifyQuerySnapshot(t, q.String())
}

func TestBuildFilterCondition_UnsupportedOperator(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpGt, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), str("1s"))
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFilterCondition_ArityErrors(t *testing.T) {
	r := newTestReader()
	tests := []struct {
		name      string
		predicate *expression.Call
	}{
		{"and with no args", call(expression.OpAnd)},
		{"not with two args", call(expression.OpNot, str("a"), str("b"))},
		{"eq with one arg", call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q strings.Builder
			_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, tt.predicate)
			require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
		})
	}
}

func TestBuildFilterCondition_BooleanCombinatorRejectsNonPredicate(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpAnd, str("not a predicate"))
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.ErrorContains(t, err, "combines predicates, not values")
}

func TestBuildFilterCondition_NotRejectsNonPredicate(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpNot, str("not a predicate"))
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.ErrorContains(t, err, "negates a predicate, not a value")
}

func TestBuildComparisonCondition_RejectsValueAgainstValue(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpEq, str("a"), str("b"))
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
}

func TestBuildFieldComparison(t *testing.T) {
	tests := []struct {
		name       string
		ref        expression.FieldRef
		value      expression.Expression
		wantColumn string
		wantValue  any
	}{
		{"span name", *fieldRef(expression.LevelSpan, expression.SpanFieldName), str("checkout"), "s.name", "checkout"},
		{"span kind server", *fieldRef(expression.LevelSpan, expression.SpanFieldKind), str("server"), "s.kind", "server"},
		{"span kind unspecified maps to empty", *fieldRef(expression.LevelSpan, expression.SpanFieldKind), str("unspecified"), "s.kind", ""},
		{"span status ok", *fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("ok"), "s.status_code", "Ok"},
		{"span status error", *fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("error"), "s.status_code", "Error"},
		{"span status unset", *fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("unset"), "s.status_code", "Unset"},
		{
			"span duration", *fieldRef(expression.LevelSpan, expression.SpanFieldDuration),
			&expression.DurationValue{Value: 2 * time.Second}, "s.duration", int64(2 * time.Second),
		},
		{"resource service", *fieldRef(expression.LevelResource, expression.ResourceFieldService), str("cart"), "s.service_name", "cart"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q strings.Builder
			args, err := buildFieldComparison(&q, 0, nil, tt.ref, tt.value)
			require.NoError(t, err)
			assert.Contains(t, q.String(), tt.wantColumn+" = ?")
			assert.Equal(t, []any{tt.wantValue}, args)
		})
	}
}

func TestBuildFieldComparison_UnsupportedLevel(t *testing.T) {
	var q strings.Builder
	_, err := buildFieldComparison(&q, 0, nil, *fieldRef(expression.LevelEvent, expression.EventFieldName), str("x"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFieldComparison_UnsupportedField(t *testing.T) {
	var q strings.Builder
	_, err := buildFieldComparison(&q, 0, nil, *fieldRef(expression.LevelSpan, expression.SpanFieldTraceID), str("x"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFieldComparison_WrongConstantType(t *testing.T) {
	var q strings.Builder
	_, err := buildFieldComparison(&q, 0, nil, *fieldRef(expression.LevelSpan, expression.SpanFieldName),
		&expression.IntValue{Value: 1})
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFieldComparison_UnsupportedFieldType(t *testing.T) {
	_, err := builtinFieldValue(expression.FieldTypeTimestamp, str("2026-01-01T00:00:00Z"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

// TestSpanStatusColumnValue_UnrecognizedWord pins that an out-of-set span.status word is
// refused rather than silently read as "unset": the query boundary already restricts span.status
// to its closed word set (RFC 0005 §5.4), so a word this reader does not recognize means a
// remote-storage client skipped that check.
func TestSpanStatusColumnValue_UnrecognizedWord(t *testing.T) {
	_, err := spanStatusColumnValue("bogus")
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
}

func TestBuildAttributeComparison_TypedConstants(t *testing.T) {
	r := newTestReader()
	tests := []struct {
		name  string
		value expression.Expression
	}{
		{"string", str("v")},
		{"int", &expression.IntValue{Value: 1}},
		{"double", &expression.DoubleValue{Value: 1.5}},
		{"bool", &expression.BoolValue{Value: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q strings.Builder
			args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil, *attrRef(expression.LevelSpan, "k"), tt.value)
			require.NoError(t, err)
			assert.Contains(t, q.String(), "arrayExists")
			assert.Len(t, args, 2)
		})
	}
}

func TestBuildAttributeComparison_UnsupportedConstantType(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	_, err := r.buildAttributeComparison(t.Context(), &q, 0, nil, *attrRef(expression.LevelSpan, "k"),
		&expression.DurationValue{Value: time.Second})
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildAttributeComparison_UnqualifiedSearchesSpanAndResourceOnly(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil, *attrRef("", "k"), str("v"))
	require.NoError(t, err)
	query := q.String()
	assert.Contains(t, query, "s.str_attributes")
	assert.Contains(t, query, "s.resource_str_attributes")
	assert.NotContains(t, query, "scope_str_attributes")
	assert.NotContains(t, query, "s.events")
	assert.NotContains(t, query, "s.links")
	assert.Len(t, args, 4) // two levels, (key, value) each
}

func TestBuildAttributeComparison_QualifiedLevels(t *testing.T) {
	tests := []struct {
		level  expression.Level
		column string
	}{
		{expression.LevelSpan, "s.str_attributes"},
		{expression.LevelResource, "s.resource_str_attributes"},
		{expression.LevelScope, "s.scope_str_attributes"},
	}
	r := newTestReader()
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			var q strings.Builder
			_, err := r.buildAttributeComparison(t.Context(), &q, 0, nil, *attrRef(tt.level, "k"), str("v"))
			require.NoError(t, err)
			assert.Contains(t, q.String(), tt.column)
		})
	}
}

func TestBuildAttributeComparison_NestedLevels(t *testing.T) {
	levels := []expression.Level{expression.LevelEvent, expression.LevelLink}
	r := newTestReader()
	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			var q strings.Builder
			_, err := r.buildAttributeComparison(t.Context(), &q, 0, nil, *attrRef(level, "k"), str("v"))
			require.NoError(t, err)
			verifyQuerySnapshot(t, q.String())
		})
	}
}

func TestBuildAttributeComparison_UnsupportedLevel(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	_, err := r.buildAttributeComparison(t.Context(), &q, 0, nil, *attrRef(expression.Level("bogus"), "k"), str("v"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildUntypedAttributeEq_ResolvesAgainstMetadata(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{{
						AttributeKey: "http.status_code",
						Type:         "int",
						Level:        "span",
					}},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	var q strings.Builder
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil,
		*attrRef(expression.LevelSpan, "http.status_code"), &expression.AnyValue{Value: "500"})
	require.NoError(t, err)
	verifyQuerySnapshot(t, q.String())
	require.Len(t, args, 2)
	assert.Equal(t, int64(500), args[1])
}

func TestBuildUntypedAttributeEq_NoMetadataFallsBackToString(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{ScanFn: scanAttributeMetadataFn()},
			},
		},
	}
	r := NewReader(driver, testReaderConfig) // metadata query succeeds but reports no rows
	var q strings.Builder
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil,
		*attrRef(expression.LevelSpan, "k"), &expression.AnyValue{Value: "v"})
	require.NoError(t, err)
	assert.Contains(t, q.String(), "s.str_attributes")
	assert.Equal(t, []any{"k", "v"}, args)
}

func TestBuildUntypedAttributeEq_MetadataQueryError(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {Err: assert.AnError},
		},
	}
	r := NewReader(driver, testReaderConfig)
	var q strings.Builder
	_, err := r.buildAttributeComparison(t.Context(), &q, 0, nil,
		*attrRef(expression.LevelSpan, "k"), &expression.AnyValue{Value: "v"})
	require.ErrorContains(t, err, "failed to get attribute metadata")
}

func TestBuildFindTraceIDsQuery_WithFilter(t *testing.T) {
	r := newTestReader()
	query := tracestore.TraceQueryParams{
		Filter:       call(expression.OpEq, fieldRef(expression.LevelResource, expression.ResourceFieldService), str("cart")),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	sqlText, args, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.NoError(t, err)
	verifyQuerySnapshot(t, sqlText)
	assert.Contains(t, args, "cart")
}

func TestBuildFindTraceIDsQuery_FilterError(t *testing.T) {
	r := newTestReader()
	query := tracestore.TraceQueryParams{
		Filter:       call(expression.OpGt, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), str("1s")),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	_, _, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFilterCondition_NotPropagatesChildError(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpNot,
		call(expression.OpGt, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), str("1s")),
	)
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildBooleanCondition_PropagatesChildError(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpAnd,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("checkout")),
		call(expression.OpGt, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), str("1s")),
	)
	_, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

// TestBuildFilterCondition_Eq_AttributeRef drives an attribute equality through the top-level
// buildFilterCondition entry point, not just buildAttributeComparison directly, so the
// OpEq -> buildComparisonCondition -> AttributeRef dispatch itself is exercised.
func TestBuildFilterCondition_Eq_AttributeRef(t *testing.T) {
	r := newTestReader()
	var q strings.Builder
	predicate := call(expression.OpEq, attrRef(expression.LevelSpan, "http.method"), str("GET"))
	args, err := r.buildFilterCondition(t.Context(), &q, 0, nil, predicate)
	require.NoError(t, err)
	assert.Contains(t, q.String(), "arrayExists")
	assert.Equal(t, []any{"http.method", "GET"}, args)
}

func TestBuiltinFieldValue_WrongTypeForKindStatusAndDuration(t *testing.T) {
	tests := []struct {
		name      string
		fieldType expression.FieldType
	}{
		{"kind", expression.FieldTypeSpanKind},
		{"status", expression.FieldTypeSpanStatus},
		{"duration", expression.FieldTypeDuration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := builtinFieldValue(tt.fieldType, &expression.IntValue{Value: 1})
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
		})
	}
}

func TestBuildUntypedAttributeEq_MultipleCandidateTypesAreOred(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{
						{AttributeKey: "http.status_code", Type: "int", Level: "span"},
						{AttributeKey: "http.status_code", Type: "str", Level: "span"},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	var q strings.Builder
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil,
		*attrRef(expression.LevelSpan, "http.status_code"), &expression.AnyValue{Value: "500"})
	require.NoError(t, err)
	query := q.String()
	assert.Contains(t, query, "s.int_attributes")
	assert.Contains(t, query, "s.str_attributes")
	assert.Contains(t, query, "OR")
	assert.Len(t, args, 4) // two (key, value) pairs, one per matched type
}

func TestBuildUntypedAttributeEq_FallbackAcrossMultipleLevels(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{ScanFn: scanAttributeMetadataFn()},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	var q strings.Builder
	// Unqualified: searches span and resource, so the no-metadata fallback ORs across both.
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil, *attrRef("", "k"), &expression.AnyValue{Value: "v"})
	require.NoError(t, err)
	query := q.String()
	assert.Contains(t, query, "s.str_attributes")
	assert.Contains(t, query, "s.resource_str_attributes")
	assert.Contains(t, query, "OR")
	assert.Equal(t, []any{"k", "v", "k", "v"}, args)
}

func TestBuildUntypedAttributeEq_FallbackAtNestedLevel(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{ScanFn: scanAttributeMetadataFn()},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	var q strings.Builder
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil,
		*attrRef(expression.LevelEvent, "exception.type"), &expression.AnyValue{Value: "TimeoutError"})
	require.NoError(t, err)
	assert.Contains(t, q.String(), "s.events")
	assert.Equal(t, []any{"exception.type", "TimeoutError"}, args)
}

func TestBuildUntypedAttributeEq_ResolvedAtNestedLevel(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{
						{AttributeKey: "retry.count", Type: "int", Level: "link"},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	var q strings.Builder
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil,
		*attrRef(expression.LevelLink, "retry.count"), &expression.AnyValue{Value: "3"})
	require.NoError(t, err)
	assert.Contains(t, q.String(), "s.links")
	assert.Equal(t, []any{"retry.count", int64(3)}, args)
}

func TestBuildUntypedAttributeEq_SkipsCandidateThatCannotParse(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					// bool cannot parse "not-a-bool", so this candidate is skipped rather than
					// producing a malformed condition.
					Data: []dbmodel.AttributeMetadata{
						{AttributeKey: "flag", Type: "bool", Level: "span"},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	var q strings.Builder
	args, err := r.buildAttributeComparison(t.Context(), &q, 0, nil,
		*attrRef(expression.LevelSpan, "flag"), &expression.AnyValue{Value: "not-a-bool"})
	require.NoError(t, err)
	// No candidate parsed, so this falls back to a literal string match instead.
	assert.Contains(t, q.String(), "s.str_attributes")
	assert.Equal(t, []any{"flag", "not-a-bool"}, args)
}

func TestCollectUntypedAttributeKeys(t *testing.T) {
	keys := pcommon.NewMap()
	collectUntypedAttributeKeys(call(expression.OpAnd,
		call(expression.OpEq, attrRef(expression.LevelSpan, "untyped"), &expression.AnyValue{Value: "v"}),
		call(expression.OpEq, attrRef(expression.LevelSpan, "typed"), str("v")),
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("v")),
	), keys)
	assert.Equal(t, 1, keys.Len())
	_, ok := keys.Get("untyped")
	assert.True(t, ok)
}

func TestCollectUntypedAttributeKeys_WrongArity(t *testing.T) {
	keys := pcommon.NewMap()
	collectUntypedAttributeKeys(call(expression.OpEq, attrRef(expression.LevelSpan, "k")), keys)
	assert.Zero(t, keys.Len())
}

// TestBuildFindTraceIDsQuery_BatchesUntypedAttributeMetadataLookups pins that a filter naming
// several untyped attributes pays for one attribute_metadata query on a cold cache, not one per
// predicate: collectUntypedAttributeKeys warms the cache for the whole tree before the lowering
// walk reaches buildUntypedAttributeEq, which then looks each key up again as a cache hit.
func TestBuildFindTraceIDsQuery_BatchesUntypedAttributeMetadataLookups(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{
						{AttributeKey: "http.status_code", Type: "int", Level: "span"},
						{AttributeKey: "retry.count", Type: "int", Level: "span"},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	query := tracestore.TraceQueryParams{
		Filter: call(expression.OpAnd,
			call(expression.OpEq, attrRef(expression.LevelSpan, "http.status_code"), &expression.AnyValue{Value: "500"}),
			call(expression.OpEq, attrRef(expression.LevelSpan, "retry.count"), &expression.AnyValue{Value: "3"}),
		),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	_, _, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.NoError(t, err)

	metadataQueries := 0
	for _, recorded := range driver.RecordedQueries {
		if strings.Contains(recorded, sql.SelectAttributeMetadata) {
			metadataQueries++
		}
	}
	assert.Equal(t, 1, metadataQueries, "both keys resolve from one query, not one each")
}

func TestAttributeTypesForLevel(t *testing.T) {
	lt := attrTypes{
		span:     []pcommon.ValueType{pcommon.ValueTypeInt},
		resource: []pcommon.ValueType{pcommon.ValueTypeStr},
		scope:    []pcommon.ValueType{pcommon.ValueTypeBool},
		event:    []pcommon.ValueType{pcommon.ValueTypeDouble},
		link:     []pcommon.ValueType{pcommon.ValueTypeInt, pcommon.ValueTypeStr},
	}
	assert.Equal(t, lt.span, attributeTypesForLevel(lt, expression.LevelSpan))
	assert.Equal(t, lt.resource, attributeTypesForLevel(lt, expression.LevelResource))
	assert.Equal(t, lt.scope, attributeTypesForLevel(lt, expression.LevelScope))
	assert.Equal(t, lt.event, attributeTypesForLevel(lt, expression.LevelEvent))
	assert.Equal(t, lt.link, attributeTypesForLevel(lt, expression.LevelLink))
}

func TestConstantKind(t *testing.T) {
	tests := []struct {
		value expression.Expression
		want  string
	}{
		{str("x"), "a string constant"},
		{&expression.IntValue{Value: 1}, "an integer constant"},
		{&expression.DoubleValue{Value: 1.5}, "a floating-point constant"},
		{&expression.BoolValue{Value: true}, "a boolean constant"},
		{&expression.DurationValue{Value: time.Second}, "a duration constant"},
		{&expression.TimestampValue{}, "a timestamp constant"},
		{&expression.NestedRef{}, "that operand"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, constantKind(tt.value))
	}
}

// TestBuildFindTraceIDsQuery_OneMetadataQueryForKeysWithNoMetadata pins that keys the metadata
// does not know still cost one query for the whole filter. Misses are not cached, so a per-predicate
// lookup would fire a query for each of them.
func TestBuildFindTraceIDsQuery_OneMetadataQueryForKeysWithNoMetadata(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data:   []dbmodel.AttributeMetadata{},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, testReaderConfig)
	query := tracestore.TraceQueryParams{
		Filter: call(expression.OpAnd,
			call(expression.OpEq, attrRef(expression.LevelSpan, "never.seen.a"), &expression.AnyValue{Value: "1"}),
			call(expression.OpEq, attrRef(expression.LevelSpan, "never.seen.b"), &expression.AnyValue{Value: "2"}),
			call(expression.OpEq, attrRef(expression.LevelSpan, "never.seen.c"), &expression.AnyValue{Value: "3"}),
		),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	_, _, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.NoError(t, err)

	metadataQueries := 0
	for _, recorded := range driver.RecordedQueries {
		if strings.Contains(recorded, sql.SelectAttributeMetadata) {
			metadataQueries++
		}
	}
	assert.Equal(t, 1, metadataQueries, "three unknown keys cost one query, not one each")
}
