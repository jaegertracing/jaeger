// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var p builder.Predicate

// call builds the trees the builder cannot spell: a wrong arity, a constant where a predicate
// belongs, or a nil operand.
func call(op expression.Operator, args ...expression.Expression) *expression.Call {
	return &expression.Call{Op: op, Args: args}
}

func unsupportedPredicate() *expression.Call {
	return call("no_such_operator", fieldRef(expression.LevelSpan, expression.SpanFieldName), str("checkout"))
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
		expression.OpNe, expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte,
		expression.OpRegex, expression.OpExists, expression.OpIn, expression.OpNotIn, expression.OpSome,
	}, caps.Operators)
}

func newTestReader() *Reader {
	return NewReader(&clickhousetest.Driver{}, NestedLayout{}, testReaderConfig)
}

// lowerFilter lowers predicate the way buildFindTraceIDsQuery does: one metadata lookup for the
// whole tree, then the lowering reads from that map.
func lowerFilter(t *testing.T, r *Reader, q *sqlBuilder, predicate *expression.Call) ([]any, error) {
	t.Helper()
	metadata, err := r.lookupFilterAttributeMetadata(t.Context(), predicate)
	if err != nil {
		return nil, err
	}
	return q.buildFilterCondition(0, nil, metadata, predicate)
}

func TestBuildFilterCondition_And(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.And(
		p.Span().Name.Eq(p.String("checkout")),
		p.Resource().Service.Eq(p.String("cart")),
	)
	args, err := lowerFilter(t, r, &q, predicate)
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, q.String())
	assert.Equal(t, []any{"checkout", "cart"}, args)
}

func TestBuildFilterCondition_Or(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.Or(
		p.Span().Name.Eq(p.String("checkout")),
		p.Span().Name.Eq(p.String("cart")),
	)
	_, err := lowerFilter(t, r, &q, predicate)
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, q.String())
}

func TestBuildFilterCondition_Not(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.Not(
		p.Span().Name.Eq(p.String("checkout")),
	)
	args, err := lowerFilter(t, r, &q, predicate)
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, q.String())
	assert.Equal(t, []any{"checkout"}, args)
}

func TestBuildFilterCondition_NestedBoolean(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.And(
		p.Or(
			p.Span().Name.Eq(p.String("a")),
			p.Span().Name.Eq(p.String("b")),
		),
		p.Not(
			p.Resource().Service.Eq(p.String("noisy")),
		),
	)
	_, err := lowerFilter(t, r, &q, predicate)
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, q.String())
}

func TestBuildFilterCondition_UnsupportedOperator(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := unsupportedPredicate()
	_, err := lowerFilter(t, r, &q, predicate)
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
			q := newNestedSQLBuilder()
			_, err := lowerFilter(t, r, &q, tt.predicate)
			require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
		})
	}
}

func TestBuildFilterCondition_BooleanCombinatorRejectsNonPredicate(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := call(expression.OpAnd, str("not a predicate"))
	_, err := lowerFilter(t, r, &q, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.ErrorContains(t, err, "combines predicates, not values")
}

func TestBuildFilterCondition_NotRejectsNonPredicate(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := call(expression.OpNot, str("not a predicate"))
	_, err := lowerFilter(t, r, &q, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.ErrorContains(t, err, "negates a predicate, not a value")
}

func TestBuildComparisonCondition_RejectsValueAgainstValue(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := call(expression.OpEq, str("a"), str("b"))
	_, err := lowerFilter(t, r, &q, predicate)
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
			q := newNestedSQLBuilder()
			args, err := q.buildFieldComparison(0, nil, expression.OpEq, tt.ref, tt.value)
			require.NoError(t, err)
			assert.Contains(t, q.String(), tt.wantColumn+" = ?")
			assert.Equal(t, []any{tt.wantValue}, args)
		})
	}
}

// TestBuildFieldComparison_UnsupportedField pins that a built-in field without a column is
// refused as an unsupported field: its level is indexed and declared, so a message that
// blames the level would be false.
func TestBuildFieldComparison_UnsupportedField(t *testing.T) {
	q := newNestedSQLBuilder()
	_, err := q.buildFieldComparison(0, nil, expression.OpEq, *fieldRef(expression.LevelScope, expression.ScopeFieldSchemaURL), str("x"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	assert.ErrorContains(t, err, `does not support the built-in field "schemaURL" of the "scope" level`)
}

func TestBuildFieldComparison_WrongConstantType(t *testing.T) {
	q := newNestedSQLBuilder()
	_, err := q.buildFieldComparison(0, nil, expression.OpEq, *fieldRef(expression.LevelSpan, expression.SpanFieldName),
		&expression.IntValue{Value: 1})
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuiltinFieldValue_TimestampUnixNanoRange(t *testing.T) {
	earliest := time.Date(1677, 9, 21, 0, 12, 43, 145224192, time.UTC)
	latest := time.Date(2262, 4, 11, 23, 47, 16, 854775807, time.UTC)
	accepted := []struct {
		instant  time.Time
		unixNano int64
	}{
		{
			instant:  earliest,
			unixNano: math.MinInt64,
		},
		{
			instant:  latest,
			unixNano: math.MaxInt64,
		},
	}
	for _, tt := range accepted {
		value, err := builtinFieldValue(expression.FieldTypeTimestamp, &expression.TimestampValue{Value: tt.instant})
		require.NoError(t, err)
		assert.Equal(t, tt.unixNano, value)
	}
	refused := []time.Time{
		earliest.Add(-time.Nanosecond),
		latest.Add(time.Nanosecond),
		time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC),
		{},
	}
	for _, instant := range refused {
		_, err := builtinFieldValue(expression.FieldTypeTimestamp, &expression.TimestampValue{Value: instant})
		require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
		require.ErrorContains(t, err, "outside the range")
	}
}

func TestBuildFieldComparison_WrongConstantTypeForTimestamp(t *testing.T) {
	_, err := builtinFieldValue(expression.FieldTypeTimestamp, str("2026-01-01T00:00:00Z"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFieldComparison_UnsupportedFieldType(t *testing.T) {
	_, err := builtinFieldValue("no-such-type", str("2026-01-01T00:00:00Z"))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFieldComparison_OrdersDuration(t *testing.T) {
	tests := []struct {
		op      expression.Operator
		wantSQL string
	}{
		{expression.OpGt, "s.duration > ?"},
		{expression.OpLt, "s.duration < ?"},
		{expression.OpGte, "s.duration >= ?"},
		{expression.OpLte, "s.duration <= ?"},
	}
	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			q := newNestedSQLBuilder()
			args, err := q.buildFieldComparison(0, nil, tt.op, *fieldRef(expression.LevelSpan, expression.SpanFieldDuration),
				&expression.DurationValue{Value: time.Second})
			require.NoError(t, err)
			assert.Equal(t, tt.wantSQL, strings.TrimSpace(q.String()))
			assert.Equal(t, []any{int64(time.Second)}, args)
		})
	}
}

// TestBuildFieldComparison_RefusesOrderingAWord pins that the ordered comparisons are refused
// for the two fields that hold words, which have no order, rather than compared as text.
func TestBuildFieldComparison_RefusesOrderingAWord(t *testing.T) {
	for _, name := range []string{expression.SpanFieldKind, expression.SpanFieldStatus} {
		t.Run(name, func(t *testing.T) {
			q := newNestedSQLBuilder()
			_, err := q.buildFieldComparison(0, nil, expression.OpGt, *fieldRef(expression.LevelSpan, name), str("m"))
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.ErrorContains(t, err, "does not order the built-in field")
		})
	}
}

func TestBuildFilterCondition_DurationRange(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.And(
		p.Span().Duration.Gte(p.Duration(5*time.Millisecond)),
		p.Span().Duration.Lte(p.Duration(40*time.Millisecond)),
	)
	args, err := lowerFilter(t, r, &q, predicate)
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, q.String())
	assert.Equal(t, []any{int64(5 * time.Millisecond), int64(40 * time.Millisecond)}, args)
}

// TestBuildFilterCondition_AttributeOrderedComparison snapshots, in one file per layout, the condition
// that each filter below lowers to, after the values that it binds.
func TestBuildFilterCondition_AttributeOrderedComparison(t *testing.T) {
	filters := []struct {
		label  string
		filter *expression.Call
	}{
		{
			label:  "an int constant orders the int values",
			filter: p.Span().Attr("retry.count").Gt(p.Int(10)),
		},
		{
			label:  "a double constant orders the double values",
			filter: p.Span().Attr("load").Lte(p.Double(0.5)),
		},
		{
			label:  "a string constant orders the string values as text",
			filter: p.Span().Attr("zone").Lt(p.String("m")),
		},
		{
			label:  "an untyped constant orders the values of the types that the metadata reports, an int and a string",
			filter: p.Span().Attr("http.status_code").Gte(500),
		},
		{
			label:  "an untyped constant with a fraction is still compared with the int values",
			filter: p.Span().Attr("http.status_code").Gt(499.5),
		},
		{
			label:  "an untyped constant that is not a number is compared with the string values only",
			filter: p.Span().Attr("http.status_code").Lt("5xx"),
		},
		{
			label:  "an untyped constant on a resource attribute that the metadata reports on the span only is false",
			filter: p.Resource().Attr("http.status_code").Gte(500),
		},
		{
			label:  "an untyped constant on an attribute that the metadata does not know is false",
			filter: p.Span().Attr("retry.count").Gt(10),
		},
		{
			label:  "an unqualified attribute searches the span and the resource",
			filter: p.Attr("retry.count").Gt(p.Int(10)),
		},
		{
			label:  "an attribute of the resource",
			filter: p.Resource().Attr("shard").Lt(p.Int(8)),
		},
		{
			label:  "an attribute of the scope",
			filter: p.Scope().Attr("build").Gte(p.Int(3)),
		},
		{
			label:  "an attribute of an event",
			filter: p.Event().Attr("attempt").Gt(p.Int(1)),
		},
		{
			label:  "an attribute of a link",
			filter: p.Link().Attr("weight").Lte(p.Double(0.5)),
		},
	}
	for _, tt := range testLayouts {
		t.Run(tt.name, func(t *testing.T) {
			var snapshot strings.Builder
			for _, filterCase := range filters {
				// The test driver returns its metadata rows once, so each filter has a reader.
				reader := newFilterMetadataReader(tt.layout)
				appendFilterSnapshot(t, &snapshot, reader, filterCase.label, filterCase.filter)
			}
			clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
		})
	}
}

// TestUntypedOrderedValue pins the Go type of the value that each stored type
// is ordered against, which a snapshot of the bind values does not show.
func TestUntypedOrderedValue(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		valueType pcommon.ValueType
		value     any
		wantOK    bool
	}{
		{
			name:      "an integer beside int values is an int64",
			raw:       "9007199254740993",
			valueType: pcommon.ValueTypeInt,
			value:     int64(9007199254740993),
			wantOK:    true,
		},
		{
			name:      "a fraction beside int values is a float64",
			raw:       "499.5",
			valueType: pcommon.ValueTypeInt,
			value:     float64(499.5),
			wantOK:    true,
		},
		{
			name:      "an integer beside double values is a float64",
			raw:       "500",
			valueType: pcommon.ValueTypeDouble,
			value:     float64(500),
			wantOK:    true,
		},
		{
			name:      "text beside string values stays text",
			raw:       "500",
			valueType: pcommon.ValueTypeStr,
			value:     "500",
			wantOK:    true,
		},
		{
			name:      "text that is not a number beside int values",
			raw:       "5xx",
			valueType: pcommon.ValueTypeInt,
		},
		{
			name:      "a bool has no order",
			raw:       "true",
			valueType: pcommon.ValueTypeBool,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tav, ok := untypedOrderedValue("k", tt.raw, tt.valueType)
			require.Equal(t, tt.wantOK, ok)
			if !ok {
				return
			}
			assert.Equal(t, typedAttributeValue{
				key:       "k",
				value:     tt.value,
				valueType: tt.valueType,
			}, tav)
		})
	}
}

func TestBuildFilterCondition_AttributeOrderedComparison_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		filter *expression.Call
		err    error
	}{
		{
			name:   "a boolean constant",
			filter: call(expression.OpGt, attrRef(expression.LevelSpan, "k"), &expression.BoolValue{Value: true}),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a duration constant",
			filter: call(expression.OpGt, attrRef(expression.LevelSpan, "k"), &expression.DurationValue{Value: time.Second}),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "an attribute of an unknown level",
			filter: call(expression.OpGt, attrRef("trace", "k"), str("v")),
			err:    tracestore.ErrFilterUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newTestReader()
			q := reader.newSQLBuilder()
			_, err := lowerFilter(t, reader, q, tt.filter)
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestBuildComparisonCondition_RejectsReferenceOperand(t *testing.T) {
	r := newTestReader()
	for name, operand := range map[string]expression.Expression{
		"attribute": attrRef(expression.LevelSpan, "k"),
		"predicate": p.Span().Name.Eq(p.String("x")),
	} {
		t.Run(name, func(t *testing.T) {
			q := newNestedSQLBuilder()
			_, err := lowerFilter(t, r, &q, p.Compare(expression.OpEq, p.Span().Name, operand))
			require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
			assert.ErrorContains(t, err, "not another reference or predicate")
		})
	}
}

// TestSpanStatusColumnValue_UnrecognizedWord pins that an out-of-set span.status word is
// refused rather than silently read as "unset": the query boundary already restricts span.status
// to its closed word set (RFC 0005 §5.4), so a word this reader does not recognize means a
// remote-storage client skipped that check.
func TestSpanStatusColumnValue_UnrecognizedWord(t *testing.T) {
	_, err := spanStatusColumnValue("bogus")
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
}

func TestAttributeEqCandidates_TypedConstants(t *testing.T) {
	r := newTestReader()
	tests := []struct {
		name       string
		value      expression.Expression
		wantColumn string
		wantArg    any
	}{
		{"string", str("v"), "s.str_attributes", "v"},
		{"int", &expression.IntValue{Value: 1}, "s.int_attributes", int64(1)},
		{"double", &expression.DoubleValue{Value: 1.5}, "s.double_attributes", 1.5},
		{"bool", &expression.BoolValue{Value: true}, "s.bool_attributes", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newNestedSQLBuilder()
			args, err := lowerFilter(t, r, &q, p.Compare(expression.OpEq, p.Span().Attr("k"), tt.value))
			require.NoError(t, err)
			assert.Contains(t, q.String(), tt.wantColumn)
			assert.Equal(t, 1, strings.Count(q.String(), "arrayExists"), "a typed constant matches its own column only")
			assert.Equal(t, []any{"k", tt.wantArg}, args)
		})
	}
}

// TestBuildFilterCondition_NilChildIsRefused pins that a nil *expression.Call inside a
// combinator is refused as invalid rather than dereferenced: the type assertion alone passes
// for it, so each site checks the pointer too.
func TestBuildFilterCondition_NilChildIsRefused(t *testing.T) {
	r := newTestReader()
	var nilCall *expression.Call
	for _, predicate := range []*expression.Call{
		call(expression.OpAnd, p.Span().Name.Eq(p.String("a")), nilCall),
		call(expression.OpNot, nilCall),
	} {
		t.Run(string(predicate.Op), func(t *testing.T) {
			q := newNestedSQLBuilder()
			_, err := lowerFilter(t, r, &q, predicate)
			require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
		})
	}
}

func TestAttributeEqCandidates_UnsupportedConstantType(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	_, err := lowerFilter(t, r, &q, p.Span().Attr("k").Eq(p.Duration(time.Second)))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildComparisonCondition_UnqualifiedSearchesSpanAndResourceOnly(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	args, err := lowerFilter(t, r, &q, p.Attr("k").Eq(p.String("v")))
	require.NoError(t, err)
	query := q.String()
	assert.Contains(t, query, "s.str_attributes")
	assert.Contains(t, query, "s.resource_str_attributes")
	assert.NotContains(t, query, "scope_str_attributes")
	assert.NotContains(t, query, "s.events")
	assert.NotContains(t, query, "s.links")
	assert.Len(t, args, 4) // two levels, (key, value) each
}

func TestBuildComparisonCondition_QualifiedLevels(t *testing.T) {
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
			q := newNestedSQLBuilder()
			args, err := lowerFilter(t, r, &q, call(expression.OpEq, attrRef(tt.level, "k"), str("v")))
			require.NoError(t, err)
			assert.Contains(t, q.String(), tt.column)
			assert.Equal(t, 1, strings.Count(q.String(), "arrayExists"), "a qualified level searches that level only")
			assert.Equal(t, []any{"k", "v"}, args)
		})
	}
}

func TestBuildComparisonCondition_NestedLevels(t *testing.T) {
	levels := []expression.Level{expression.LevelEvent, expression.LevelLink}
	r := newTestReader()
	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			q := newNestedSQLBuilder()
			_, err := lowerFilter(t, r, &q, call(expression.OpEq, attrRef(level, "k"), str("v")))
			require.NoError(t, err)
			clickhousetest.VerifyQuerySnapshot(t, q.String())
		})
	}
}

func TestBuildComparisonCondition_UnsupportedLevel(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	_, err := lowerFilter(t, r, &q, call(expression.OpEq, attrRef(expression.Level("bogus"), "k"), str("v")))
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestUntypedAttributeCandidates_ResolvesAgainstMetadata(t *testing.T) {
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
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	q := newNestedSQLBuilder()
	args, err := lowerFilter(t, r, &q, p.Span().Attr("http.status_code").Eq("500"))
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, q.String())
	require.Len(t, args, 2)
	assert.Equal(t, int64(500), args[1])
}

// TestBuildFilterCondition_UnknownAttributeKey snapshots the condition of an equality with an
// untyped constant on a key without a row in the attribute metadata, which matches nothing.
func TestBuildFilterCondition_UnknownAttributeKey(t *testing.T) {
	filters := []struct {
		label  string
		filter *expression.Call
	}{
		{
			label:  "a span attribute",
			filter: p.Span().Attr("k").Eq("v"),
		},
		{
			label:  "an unqualified attribute",
			filter: p.Attr("k").Eq("v"),
		},
		{
			label:  "an event attribute",
			filter: p.Event().Attr("k").Eq("v"),
		},
	}
	var snapshot strings.Builder
	for _, filterCase := range filters {
		driver := &clickhousetest.Driver{
			QueryResponses: map[string]*clickhousetest.QueryResponse{
				sql.SelectAttributeMetadata: {
					Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{ScanFn: scanAttributeMetadataFn()},
				},
			},
		}
		r := NewReader(driver, NestedLayout{}, testReaderConfig)
		q := newNestedSQLBuilder()
		args, err := lowerFilter(t, r, &q, filterCase.filter)
		require.NoError(t, err, filterCase.label)
		fmt.Fprintf(&snapshot, "\n-- %s\n-- binds %v%s", filterCase.label, args, q.String())
	}
	clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
}

func TestLookupFilterAttributeMetadata_QueryError(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {Err: assert.AnError},
		},
	}
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	q := newNestedSQLBuilder()
	_, err := lowerFilter(t, r, &q, p.Span().Attr("k").Eq("v"))
	require.ErrorContains(t, err, "failed to get attribute metadata")
}

func TestBuildFindTraceIDsQuery_WithFilter(t *testing.T) {
	r := newTestReader()
	query := tracestore.TraceQueryParams{
		Filter:       p.Resource().Service.Eq(p.String("cart")),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	sqlText, args, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, sqlText)
	assert.Equal(t, []any{"cart", time.Unix(0, 0), time.Unix(100, 0), testReaderConfig.DefaultSearchDepth}, args)
}

// TestBuildFindTraceIDsQuery_WithNestedFilter snapshots the whole query for a boolean tree with
// an untyped attribute resolved through metadata, so the indentation and parenthesis placement
// under the outer AND are pinned for the path a real query takes, not only for a single leaf.
func TestBuildFindTraceIDsQuery_WithNestedFilter(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{
						{AttributeKey: "http.status_code", Type: "int", Level: "span"},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	query := tracestore.TraceQueryParams{
		Filter: p.And(
			p.Or(
				p.Span().Name.Eq(p.String("checkout")),
				p.Span().Attr("http.status_code").Eq("500"),
			),
			p.Not(
				p.Span().Duration.Gt(p.Duration(time.Second)),
			),
		),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	sqlText, args, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, sqlText)
	assert.Equal(t, []any{"checkout", "http.status_code", int64(500), int64(time.Second), time.Unix(0, 0), time.Unix(100, 0), testReaderConfig.DefaultSearchDepth}, args)
}

func TestBuildFindTraceIDsQuery_FilterBesideLegacyFields(t *testing.T) {
	r := newTestReader()
	query := tracestore.TraceQueryParams{
		ServiceName:  "cart",
		Filter:       p.Span().Name.Eq(p.String("checkout")),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	_, _, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
}

func TestBuildFindTraceIDsQuery_MetadataError(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {Err: assert.AnError},
		},
	}
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	query := tracestore.TraceQueryParams{
		Filter:       p.Span().Attr("k").Eq("v"),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	_, _, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.ErrorContains(t, err, "failed to get attribute metadata")
}

func TestBuildFindTraceIDsQuery_FilterError(t *testing.T) {
	r := newTestReader()
	query := tracestore.TraceQueryParams{
		Filter:       unsupportedPredicate(),
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Unix(100, 0),
	}
	_, _, err := r.buildFindTraceIDsQuery(t.Context(), query)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildFilterCondition_NotPropagatesChildError(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.Not(
		unsupportedPredicate(),
	)
	_, err := lowerFilter(t, r, &q, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestBuildBooleanCondition_PropagatesChildError(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.And(
		p.Span().Name.Eq(p.String("checkout")),
		unsupportedPredicate(),
	)
	_, err := lowerFilter(t, r, &q, predicate)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

// TestBuildFilterCondition_Eq_AttributeRef drives an attribute equality through the top-level
// buildFilterCondition entry point, not just attributeComparison directly, so the
// OpEq -> comparisonCondition -> AttributeRef dispatch itself is exercised.
func TestBuildFilterCondition_Eq_AttributeRef(t *testing.T) {
	r := newTestReader()
	q := newNestedSQLBuilder()
	predicate := p.Span().Attr("http.method").Eq(p.String("GET"))
	args, err := lowerFilter(t, r, &q, predicate)
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

func TestUntypedAttributeCandidates_MultipleCandidateTypesAreOred(t *testing.T) {
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
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	q := newNestedSQLBuilder()
	args, err := lowerFilter(t, r, &q, p.Span().Attr("http.status_code").Eq("500"))
	require.NoError(t, err)
	query := q.String()
	assert.Contains(t, query, "s.int_attributes")
	assert.Contains(t, query, "s.str_attributes")
	assert.Contains(t, query, "OR")
	assert.Len(t, args, 4) // two (key, value) pairs, one per matched type
}

func TestUntypedAttributeCandidates_ResolvedAtNestedLevel(t *testing.T) {
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
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	q := newNestedSQLBuilder()
	args, err := lowerFilter(t, r, &q, p.Link().Attr("retry.count").Eq("3"))
	require.NoError(t, err)
	assert.Contains(t, q.String(), "s.links")
	assert.Equal(t, []any{"retry.count", int64(3)}, args)
}

func TestUntypedAttributeCandidates_SkipsCandidateThatCannotParse(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					// bool cannot parse "not-a-bool", so this candidate is skipped rather than
					// producing a malformed condition, and no stored value can equal the constant.
					Data: []dbmodel.AttributeMetadata{
						{AttributeKey: "flag", Type: "bool", Level: "span"},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	q := newNestedSQLBuilder()
	args, err := lowerFilter(t, r, &q, p.Span().Attr("flag").Eq("not-a-bool"))
	require.NoError(t, err)
	assert.Equal(t, "\n(\n\tfalse\n)", q.String())
	assert.Empty(t, args)
}

func TestCollectMetadataKeys(t *testing.T) {
	keys := pcommon.NewMap()
	collectMetadataKeys(p.And(
		p.Span().Attr("untyped").Eq("v"),
		p.Span().Attr("typed").Eq(p.String("v")),
		p.Span().Name.Eq(p.String("v")),
	), keys)
	assert.Equal(t, 1, keys.Len())
	_, ok := keys.Get("untyped")
	assert.True(t, ok)
}

// TestBuildFilterCondition_AttributeOrderedComparison_HoistedColumn snapshots the condition of
// an ordered comparison on the resource attribute "shard", which the table hoists into a column.
func TestBuildFilterCondition_AttributeOrderedComparison_HoistedColumn(t *testing.T) {
	var snapshot strings.Builder
	appendFilterSnapshot(t, &snapshot, newHoistedColumnReader(), "an ordered comparison on a hoisted attribute", p.Resource().Attr("shard").Gt(p.Int(7)))
	clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
}

func TestCollectMetadataKeys_WrongArity(t *testing.T) {
	keys := pcommon.NewMap()
	collectMetadataKeys(call(expression.OpEq, attrRef(expression.LevelSpan, "k")), keys)
	assert.Zero(t, keys.Len())
}

// TestBuildFindTraceIDsQuery_BatchesUntypedAttributeMetadataLookups pins that a filter naming
// several untyped attributes pays for one attribute_metadata query on a cold cache, not one per
// predicate: lookupFilterAttributeMetadata collects the keys of the whole tree into one query and the
// lowering reads the resulting map.
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
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	query := tracestore.TraceQueryParams{
		Filter: p.And(
			p.Span().Attr("http.status_code").Eq("500"),
			p.Span().Attr("retry.count").Eq("3"),
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
	r := NewReader(driver, NestedLayout{}, testReaderConfig)
	query := tracestore.TraceQueryParams{
		Filter: p.And(
			p.Span().Attr("never.seen.a").Eq("1"),
			p.Span().Attr("never.seen.b").Eq("2"),
			p.Span().Attr("never.seen.c").Eq("3"),
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

// TestAttributeEqCandidates_HoistedColumn checks that a negated equality keeps the
// IS NOT NULL test inside the NOT, and that an unqualified reference reads the column at the
// span level and the typed group at the resource level.
func TestAttributeEqCandidates_HoistedColumn(t *testing.T) {
	reader := newHoistedColumnReader()

	t.Run("negation", func(t *testing.T) {
		q := reader.newSQLBuilder()
		args, err := lowerFilter(t, reader, q, p.Not(p.Span().Attr("model").Eq(p.String("x"))))
		require.NoError(t, err)
		clickhousetest.VerifyQuerySnapshot(t, q.String())
		assert.Equal(t, []any{"x"}, args)
	})

	t.Run("unqualified reference", func(t *testing.T) {
		q := reader.newSQLBuilder()
		args, err := lowerFilter(t, reader, q, p.Attr("model").Eq(p.String("x")))
		require.NoError(t, err)
		clickhousetest.VerifyQuerySnapshot(t, q.String())
		assert.Equal(t, []any{"x", "model", "x"}, args)
	})
}

// TestUntypedAttributeCandidates_HoistedColumn checks that a filter value whose type comes from
// the attribute metadata is converted to the type of the hoisted column.
func TestUntypedAttributeCandidates_HoistedColumn(t *testing.T) {
	reader := newHoistedColumnReader()
	q := reader.newSQLBuilder()

	args, err := lowerFilter(t, reader, q, p.Resource().Attr("shard").Eq("7"))

	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, q.String())
	assert.Equal(t, []any{int64(7)}, args)
}

// newFilterMetadataReader returns a reader of a table in the layout, whose attribute
// metadata says that the span attribute "http.status_code" is stored as an int and as a string,
// the span attribute "payload" as a map and as bytes, and the attribute "zone" as a string on
// the span, the resource, the event and the link.
func newFilterMetadataReader(layout AttributeLayout) *Reader {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{
						{
							AttributeKey: "http.status_code",
							Type:         "int",
							Level:        "span",
						},
						{
							AttributeKey: "http.status_code",
							Type:         "str",
							Level:        "span",
						},
						{
							AttributeKey: "payload",
							Type:         "map",
							Level:        "span",
						},
						{
							AttributeKey: "payload",
							Type:         "bytes",
							Level:        "span",
						},
						{
							AttributeKey: "zone",
							Type:         "str",
							Level:        "span",
						},
						{
							AttributeKey: "zone",
							Type:         "str",
							Level:        "resource",
						},
						{
							AttributeKey: "zone",
							Type:         "str",
							Level:        "event",
						},
						{
							AttributeKey: "zone",
							Type:         "str",
							Level:        "link",
						},
					},
					ScanFn: scanAttributeMetadataFn(),
				},
			},
		},
	}
	return NewReader(driver, layout, testReaderConfig)
}

// TestBuildFilterCondition_ExistsNeAndMembership snapshots, in one file per layout, the
// condition that each filter below lowers to, after the values that it binds.
func TestBuildFilterCondition_ExistsNeAndMembership(t *testing.T) {
	filters := []struct {
		label  string
		filter *expression.Call
	}{
		{
			label:  "exists on a span attribute that the metadata does not know is false",
			filter: p.Span().Attr("tier").Exists(),
		},
		{
			label:  "exists on an unqualified attribute that the metadata reports as a string on the span and on the resource",
			filter: p.Attr("zone").Exists(),
		},
		{
			label:  "exists on an event attribute that the metadata reports as a string",
			filter: p.Event().Attr("zone").Exists(),
		},
		{
			label:  "exists on a span attribute that the metadata reports as an int and as a string",
			filter: p.Span().Attr("http.status_code").Exists(),
		},
		{
			label:  "exists on an unqualified attribute that the metadata reports on the span only",
			filter: p.Attr("http.status_code").Exists(),
		},
		{
			label:  "exists on a resource attribute that the metadata reports on the span only is false",
			filter: p.Resource().Attr("http.status_code").Exists(),
		},
		{
			label:  "exists on a span attribute that the metadata reports as bytes and as a map",
			filter: p.Span().Attr("payload").Exists(),
		},
		{
			label:  "ne on a resource attribute that the metadata reports on the span only is false",
			filter: p.Resource().Attr("http.status_code").Ne("200"),
		},
		{
			label:  "exists on a field that every span has",
			filter: p.Span().Name.Exists(),
		},
		{
			label:  "exists on a field that a span can lack",
			filter: p.Resource().Service.Exists(),
		},
		{
			label:  "ne on a span attribute with a string constant",
			filter: p.Span().Attr("zone").Ne(p.String("us-east")),
		},
		{
			label:  "ne on an unqualified attribute that the metadata does not know is false",
			filter: p.Attr("tier").Ne("us-east"),
		},
		{
			label:  "ne on an unqualified attribute with an untyped constant, for an attribute that the metadata reports as a string on the span and on the resource",
			filter: p.Attr("zone").Ne("us-east"),
		},
		{
			label:  "ne on a span attribute that the metadata reports as an int and as a string",
			filter: p.Span().Attr("http.status_code").Ne("200"),
		},
		{
			label:  "ne on a link attribute",
			filter: p.Link().Attr("zone").Ne(p.String("us-east")),
		},
		{
			label:  "ne on a field that every span has",
			filter: p.Span().Status.Ne(p.String("error")),
		},
		{
			label:  "ne on a field that a span can lack",
			filter: p.Resource().Service.Ne(p.String("cart")),
		},
		{
			label:  "in on a span attribute with an untyped list, for an attribute that the metadata reports as an int and as a string",
			filter: p.Span().Attr("http.status_code").In("200", "5xx"),
		},
		{
			label:  "in on a resource attribute with a list of ints",
			filter: p.Resource().Attr("shard").In(p.List(expression.ValueTypeInt, 1, 2)),
		},
		{
			label:  "in on a string field",
			filter: p.Span().Name.In("GET /cart", "POST /cart"),
		},
		{
			label:  "in on the span kind",
			filter: p.Span().Kind.In("server", "unspecified"),
		},
		{
			label:  "in on the duration",
			filter: p.Span().Duration.In(time.Second, 2*time.Second),
		},
		{
			label:  "in on a field that a span can lack, with an empty element",
			filter: p.Resource().Service.In(""),
		},
		{
			label:  "not_in on an unqualified attribute",
			filter: p.Attr("zone").NotIn("us-east", "eu-west"),
		},
		{
			label:  "not_in on a field that every span has",
			filter: p.Span().Name.NotIn("GET /cart"),
		},
		{
			label:  "not_in on a field that a span can lack",
			filter: p.Resource().Service.NotIn("cart", "checkout"),
		},
	}
	for _, tt := range testLayouts {
		t.Run(tt.name, func(t *testing.T) {
			var snapshot strings.Builder
			for _, filterCase := range filters {
				// The test driver returns its metadata rows once, so each filter has a reader.
				reader := newFilterMetadataReader(tt.layout)
				appendFilterSnapshot(t, &snapshot, reader, filterCase.label, filterCase.filter)
			}
			clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
		})
	}
}

func TestBuildFilterCondition_ExistsNeAndMembership_Refusals(t *testing.T) {
	list := func(valueType expression.ValueType, values ...string) *expression.List {
		return &expression.List{
			Values: values,
			Type:   valueType,
		}
	}
	tests := []struct {
		name   string
		filter *expression.Call
		err    error
	}{
		{
			name:   "exists with two operands",
			filter: call(expression.OpExists, attrRef(expression.LevelSpan, "k"), str("v")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "exists on a constant",
			filter: call(expression.OpExists, str("v")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "exists on a field without a column mapping",
			filter: call(expression.OpExists, fieldRef(expression.LevelScope, expression.ScopeFieldSchemaURL)),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "exists on an attribute of an unknown level",
			filter: call(expression.OpExists, attrRef("trace", "k")),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "ne on an attribute of an unknown level",
			filter: call(expression.OpNe, attrRef("trace", "k"), str("v")),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "ne on an attribute with a duration constant",
			filter: call(expression.OpNe, attrRef(expression.LevelSpan, "k"), p.Duration(time.Second)),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "in with one operand",
			filter: call(expression.OpIn, attrRef(expression.LevelSpan, "k")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "in with a scalar where the list belongs",
			filter: call(expression.OpIn, attrRef(expression.LevelSpan, "k"), str("v")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "in with an empty list",
			filter: call(expression.OpIn, attrRef(expression.LevelSpan, "k"), list("")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "in with a nil list",
			filter: call(expression.OpIn, attrRef(expression.LevelSpan, "k"), (*expression.List)(nil)),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "in on a constant",
			filter: call(expression.OpIn, str("v"), list("", "v")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "in on an attribute of an unknown level",
			filter: call(expression.OpIn, attrRef("trace", "k"), list("", "v")),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "in on an attribute with an element that is not of the type of the list",
			filter: call(expression.OpIn, attrRef(expression.LevelSpan, "k"), list(expression.ValueTypeInt, "one")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "not_in on a field without a column mapping",
			filter: call(expression.OpNotIn, fieldRef(expression.LevelResource, expression.ResourceFieldSchemaURL), list("", "v")),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "in on the duration with an element that is not a duration",
			filter: call(expression.OpIn, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), list("", "soon")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "in on a string field with a list of ints",
			filter: call(expression.OpIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), list(expression.ValueTypeInt, "1")),
			err:    tracestore.ErrFilterUnsupported,
		},
	}
	reader := newFilterMetadataReader(NestedLayout{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newNestedSQLBuilder()
			_, err := lowerFilter(t, reader, &q, tt.filter)
			require.ErrorIs(t, err, tt.err)
		})
	}
}

// TestCollectMetadataKeys_Membership checks that an untyped list names a key to look up, that
// a typed list names none, and that exists, ne and not_in name their key whatever the constant,
// because their presence test reads the metadata.
func TestCollectMetadataKeys_Membership(t *testing.T) {
	keys := pcommon.NewMap()
	collectMetadataKeys(p.And(
		p.Span().Attr("ne-untyped").Ne("v"),
		p.Span().Attr("in-untyped").In("v"),
		p.Span().Attr("not-in-untyped").NotIn("v"),
		p.Span().Attr("in-typed").In(p.List(expression.ValueTypeString, "v")),
		p.Span().Attr("ne-typed").Ne(p.String("v")),
		p.Span().Attr("exists").Exists(),
		p.Span().Name.Exists(),
		call(expression.OpExists),
	), keys)
	assert.Equal(t, map[string]any{
		"ne-untyped":     "",
		"in-untyped":     "",
		"not-in-untyped": "",
		"ne-typed":       "",
		"exists":         "",
	}, keys.AsRaw())
}

// TestBuildFilterCondition_BuiltinFields snapshots the condition that each filter below
// lowers to, after the values that it binds. A built-in field has the same column in every
// attribute layout, so one layout is enough.
func TestBuildFilterCondition_BuiltinFields(t *testing.T) {
	instant := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	filters := []struct {
		label  string
		filter *expression.Call
	}{
		{
			label:  "eq on the trace ID",
			filter: p.Span().TraceID.Eq("00000000000000000000000000000f01"),
		},
		{
			label:  "eq on the span ID",
			filter: p.Span().SpanID.Eq("0000000000000f01"),
		},
		{
			label:  "exists on the parent span ID, which a root span lacks",
			filter: p.Span().ParentSpanID.Exists(),
		},
		{
			label:  "eq on the trace state",
			filter: p.Span().TraceState.Eq("congo=t61rcWkgMzE"),
		},
		{
			label:  "ne on the status message, which a span can lack",
			filter: p.Span().StatusMessage.Ne("timeout"),
		},
		{
			label:  "lt on the status message, which a span can lack",
			filter: p.Span().StatusMessage.Lt("timeout"),
		},
		{
			label:  "gt on the span name orders text",
			filter: p.Span().Name.Gt("m"),
		},
		{
			label:  "gt on the start time",
			filter: p.Span().StartTime.Gt(instant),
		},
		{
			label:  "lte on the end time, which is computed",
			filter: p.Span().EndTime.Lte(instant),
		},
		{
			label:  "in on the start time",
			filter: p.Span().StartTime.In(instant, instant.Add(time.Second)),
		},
		{
			label:  "eq on the scope name",
			filter: p.Scope().Name.Eq("worker-scope"),
		},
		{
			label:  "not_in on the scope version, which a span can lack",
			filter: p.Scope().Version.NotIn("1.0", "2.0"),
		},
		{
			label:  "eq on the name of an event",
			filter: p.Event().Name.Eq("exception"),
		},
		{
			label:  "ne on the name of an event",
			filter: p.Event().Name.Ne("exception"),
		},
		{
			label:  "exists on the name of an event",
			filter: p.Event().Name.Exists(),
		},
		{
			label:  "in on the name of an event",
			filter: p.Event().Name.In("exception", "retry"),
		},
		{
			label:  "not_in on the name of an event",
			filter: p.Event().Name.NotIn("exception", "retry"),
		},
		{
			label:  "gt on the time of an event",
			filter: p.Event().Time.Gt(instant),
		},
		{
			label:  "gt on the time of an event since the start of its span, which is computed",
			filter: p.Event().TimeSinceStart.Gt(50 * time.Microsecond),
		},
		{
			label:  "eq on the trace ID of a link",
			filter: p.Link().TraceID.Eq("00000000000000000000000000000f01"),
		},
		{
			label:  "eq on the span ID of a link",
			filter: p.Link().SpanID.Eq("0000000000000f01"),
		},
		{
			label:  "exists on the trace state of a link, which a link can lack",
			filter: p.Link().TraceState.Exists(),
		},
		{
			label:  "ne on the trace state of a link",
			filter: p.Link().TraceState.Ne("congo=t61rcWkgMzE"),
		},
		{
			label:  "lt on the trace state of a link",
			filter: p.Link().TraceState.Lt("congo=u"),
		},
		{
			label:  "in on the trace state of a link",
			filter: p.Link().TraceState.In("congo=t61rcWkgMzE"),
		},
	}
	reader := newTestReader()
	var snapshot strings.Builder
	for _, filterCase := range filters {
		// The query boundary reads each constant as the type of its field before the reader
		// sees the filter.
		filter, err := tracestore.ResolveFilterConstants(filterCase.filter)
		require.NoError(t, err, filterCase.label)
		appendFilterSnapshot(t, &snapshot, reader, filterCase.label, filter)
	}
	clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
}

// appendFilterSnapshot lowers filter and appends to snapshot its label, the values that it
// binds and its condition.
func appendFilterSnapshot(t *testing.T, snapshot *strings.Builder, reader *Reader, label string, filter *expression.Call) {
	t.Helper()
	q := reader.newSQLBuilder()
	args, err := lowerFilter(t, reader, q, filter)
	require.NoError(t, err, label)
	fmt.Fprintf(snapshot, "\n-- %s\n-- binds %v%s", label, args, q.String())
}

// TestBuildFilterCondition_Regex snapshots, for each attribute layout, the condition that
// each filter below lowers to, after the values that it binds.
func TestBuildFilterCondition_Regex(t *testing.T) {
	filters := []struct {
		label  string
		filter *expression.Call
	}{
		{
			label:  "a pattern on the span name",
			filter: p.Span().Name.Matches("^GET /cart"),
		},
		{
			label:  "a pattern with a dot clears the flag that lets the dot match a newline",
			filter: p.Span().Name.Matches("^GET /c.rt|^POST"),
		},
		{
			label:  "a pattern whose dot matches a newline is sent as written",
			filter: p.Span().Name.Matches("(?s)^GET.*cart"),
		},
		{
			label:  "a pattern that is a typed string",
			filter: p.Compare(expression.OpRegex, p.Span().Name, p.String("^GET /cart")),
		},
		{
			label:  "a pattern on a field that a span can lack",
			filter: p.Span().StatusMessage.Matches("time.?out"),
		},
		{
			label:  "a pattern on the span kind matches the word, not the stored value",
			filter: p.Span().Kind.Matches("^(server|unspecified)$"),
		},
		{
			label:  "a pattern on the span status matches the word, not the stored value",
			filter: p.Span().Status.Matches("^(error|unset)$"),
		},
		{
			label:  "a pattern on the name of an event",
			filter: p.Event().Name.Matches("^cache"),
		},
		{
			label:  "a pattern on the trace state of a link, which a link can lack",
			filter: p.Link().TraceState.Matches("^congo="),
		},
		{
			label:  "a pattern on a span attribute",
			filter: p.Span().Attr("http.route").Matches("^/api/"),
		},
		{
			label:  "a pattern on an unqualified attribute searches the span and the resource",
			filter: p.Attr("zone").Matches("^us-"),
		},
		{
			label:  "a pattern on a scope attribute",
			filter: p.Scope().Attr("library.language").Matches("^go"),
		},
		{
			label:  "a pattern on a link attribute",
			filter: p.Link().Attr("zone").Matches("^us-"),
		},
		{
			label:  "a pattern on an event attribute",
			filter: p.Event().Attr("exception.type").Matches("Error$"),
		},
	}
	for _, tt := range testLayouts {
		t.Run(tt.name, func(t *testing.T) {
			var snapshot strings.Builder
			for _, filterCase := range filters {
				reader := newFilterMetadataReader(tt.layout)
				appendFilterSnapshot(t, &snapshot, reader, filterCase.label, filterCase.filter)
			}
			clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
		})
	}
}

// TestBuildFilterCondition_Regex_HoistedColumn snapshots the condition of a pattern on the
// span attribute "model", which the table hoists into a column.
func TestBuildFilterCondition_Regex_HoistedColumn(t *testing.T) {
	var snapshot strings.Builder
	appendFilterSnapshot(t, &snapshot, newHoistedColumnReader(), "a pattern on a hoisted attribute", p.Span().Attr("model").Matches("^claude"))
	clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
}

func TestBuildFilterCondition_Regex_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		filter *expression.Call
		err    error
	}{
		{
			name:   "one operand",
			filter: call(expression.OpRegex, attrRef(expression.LevelSpan, "k")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a pattern that is not a string",
			filter: call(expression.OpRegex, attrRef(expression.LevelSpan, "k"), &expression.IntValue{Value: 1}),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a pattern that does not parse",
			filter: call(expression.OpRegex, attrRef(expression.LevelSpan, "k"), str("a(")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a constant as the subject",
			filter: call(expression.OpRegex, str("v"), str("v")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a field that holds a duration",
			filter: call(expression.OpRegex, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), str("1")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a field without a column",
			filter: call(expression.OpRegex, fieldRef(expression.LevelScope, expression.ScopeFieldSchemaURL), str("v")),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "an attribute of an unknown level",
			filter: call(expression.OpRegex, attrRef("trace", "k"), str("v")),
			err:    tracestore.ErrFilterUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newTestReader()
			q := reader.newSQLBuilder()
			_, err := lowerFilter(t, reader, q, tt.filter)
			require.ErrorIs(t, err, tt.err)
		})
	}
}

// TestBuildFilterCondition_Some snapshots, in one file per layout, the condition that each
// filter below lowers to, after the values that it binds.
func TestBuildFilterCondition_Some(t *testing.T) {
	filters := []struct {
		label  string
		filter *expression.Call
	}{
		{
			label: "two fields of one event",
			filter: p.Some(p.Event(), p.And(
				p.Event().Name.Eq("exception"),
				p.Event().TimeSinceStart.Gt(50*time.Microsecond),
			)),
		},
		{
			label: "a field and an attribute of one event",
			filter: p.Some(p.Event(), p.And(
				p.Event().Name.Eq("exception"),
				p.Event().Attr("exception.type").Eq(p.String("IOError")),
			)),
		},
		{
			label: "the bound level beside the level of the span",
			filter: p.Some(p.Event(), p.And(
				p.Event().Name.Matches("^cache"),
				p.Span().Name.Eq("GET /cart"),
				p.Span().Attr("zone").Eq(p.String("us-east")),
			)),
		},
		{
			label: "exists, ne and not_in on one link",
			filter: p.Some(p.Link(), p.And(
				p.Link().TraceState.Exists(),
				p.Link().TraceState.Ne("congo=t61rcWkgMzE"),
				p.Link().Attr("zone").Exists(),
				p.Link().Attr("zone").NotIn(p.List(expression.ValueTypeString, "us-east", "eu-west")),
			)),
		},
		{
			label: "an attribute of an event inside a predicate over one link searches every event",
			filter: p.Some(p.Link(), p.And(
				p.Link().TraceID.Eq("00000000000000000000000000000f01"),
				p.Event().Attr("attempt").Gt(p.Int(1)),
			)),
		},
		{
			label:  "every event is named, by a negation around a negated predicate",
			filter: p.Not(p.Some(p.Event(), p.Not(p.Event().Name.Exists()))),
		},
		{
			label: "a level is bound only inside its own predicate",
			filter: p.And(
				p.Some(p.Event(), p.Event().Name.Eq("exception")),
				p.Event().Name.Eq("retry"),
			),
		},
	}
	for _, tt := range testLayouts {
		t.Run(tt.name, func(t *testing.T) {
			var snapshot strings.Builder
			for _, filterCase := range filters {
				// The test driver returns its metadata rows once, so each filter has a reader.
				reader := newFilterMetadataReader(tt.layout)
				// The query boundary reads each constant as the type of its field before the
				// reader sees the filter.
				filter, err := tracestore.ResolveFilterConstants(filterCase.filter)
				require.NoError(t, err, filterCase.label)
				appendFilterSnapshot(t, &snapshot, reader, filterCase.label, filter)
			}
			clickhousetest.VerifyQuerySnapshot(t, snapshot.String())
		})
	}
}

func TestBuildFilterCondition_Some_Refusals(t *testing.T) {
	events := &expression.NestedRef{Level: expression.LevelEvent}
	links := &expression.NestedRef{Level: expression.LevelLink}
	eventNameIsException := call(expression.OpEq, fieldRef(expression.LevelEvent, expression.EventFieldName), str("exception"))
	tests := []struct {
		name   string
		filter *expression.Call
		err    error
	}{
		{
			name:   "one operand",
			filter: call(expression.OpSome, events),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a field in place of the collection",
			filter: call(expression.OpSome, fieldRef(expression.LevelEvent, expression.EventFieldName), eventNameIsException),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a collection of a level with one value per span",
			filter: call(expression.OpSome, &expression.NestedRef{Level: expression.LevelSpan}, eventNameIsException),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "a value in place of the predicate",
			filter: call(expression.OpSome, events, str("v")),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "nested over the same level",
			filter: call(expression.OpSome, events, call(expression.OpSome, events, eventNameIsException)),
			err:    tracestore.ErrFilterInvalid,
		},
		{
			name:   "nested over the other level",
			filter: call(expression.OpSome, events, call(expression.OpSome, links, eventNameIsException)),
			err:    tracestore.ErrFilterUnsupported,
		},
		{
			name:   "a predicate that is refused",
			filter: call(expression.OpSome, events, unsupportedPredicate()),
			err:    tracestore.ErrFilterUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newTestReader()
			q := reader.newSQLBuilder()
			_, err := lowerFilter(t, reader, q, tt.filter)
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestCollectMetadataKeys_Some(t *testing.T) {
	keys := pcommon.NewMap()
	collectMetadataKeys(p.Some(p.Event(), p.Event().Attr("untyped").Eq("v")), keys)
	assert.Equal(t, map[string]any{"untyped": ""}, keys.AsRaw())
}
