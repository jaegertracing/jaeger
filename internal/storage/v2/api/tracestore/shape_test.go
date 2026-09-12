// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// tag builds a predicate on an unqualified attribute, whose constant declares no type because a
// tag never did.
func tag(op expression.Operator, key string, value string) *expression.Call {
	return compare(op, &expression.AttributeRef{Key: key}, &expression.AnyValue{Value: value})
}

// text builds a predicate on a built-in field against a text constant.
func text(op expression.Operator, level expression.Level, name string, value string) *expression.Call {
	return compare(op,
		&expression.FieldRef{Level: level, Name: name},
		&expression.StringValue{Value: value})
}

// bound builds one of the inclusive duration bounds on span.duration.
func bound(op expression.Operator, d time.Duration) *expression.Call {
	return compare(op,
		&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
		&expression.DurationValue{Value: d})
}

func compare(op expression.Operator, args ...expression.Expression) *expression.Call {
	return &expression.Call{Op: op, Args: args}
}

func filterQuery(filter *expression.Call) TraceQueryParams {
	return TraceQueryParams{Attributes: pcommon.NewMap(), Filter: filter}
}

func attributes(pairs map[string]string) pcommon.Map {
	m := pcommon.NewMap()
	for k, v := range pairs {
		m.PutStr(k, v)
	}
	return m
}

// TestToLegacyShape_Converts covers the backends that do not evaluate a
// filter yet: the query service expresses what it can in the predicate fields they already
// serve, so a filter query returns what the equivalent legacy query would.
func TestToLegacyShape_Converts(t *testing.T) {
	tests := []struct {
		name     string
		filter   *expression.Call
		expected TraceQueryParams
	}{
		{
			name:     "an unqualified attribute becomes a tag",
			filter:   tag(expression.OpEq, "http.status_code", "500"),
			expected: TraceQueryParams{Attributes: attributes(map[string]string{"http.status_code": "500"})},
		},
		{
			name: "a level-qualified attribute widens to an unqualified tag",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "k8s.pod.name", Level: expression.LevelResource},
				&expression.AnyValue{Value: "cart-0"}),
			expected: TraceQueryParams{Attributes: attributes(map[string]string{"k8s.pod.name": "cart-0"})},
		},
		{
			name: "an event-level attribute widens too",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "exception.type", Level: expression.LevelEvent},
				&expression.AnyValue{Value: "IOError"}),
			expected: TraceQueryParams{Attributes: attributes(map[string]string{"exception.type": "IOError"})},
		},
		{
			// A tag holds whatever spelling was stored, so a text constant narrows to it and an
			// untyped one leaves the type open; the legacy map carries a string either way.
			name: "an attribute compared against a text constant",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "http.method"},
				&expression.StringValue{Value: "GET"}),
			expected: TraceQueryParams{Attributes: attributes(map[string]string{"http.method": "GET"})},
		},
		{
			name:     "the service built-in becomes the service name",
			filter:   text(expression.OpEq, expression.LevelResource, expression.ResourceFieldService, "cart"),
			expected: TraceQueryParams{ServiceName: "cart", Attributes: pcommon.NewMap()},
		},
		{
			name:     "the span name built-in becomes the operation name",
			filter:   text(expression.OpEq, expression.LevelSpan, expression.SpanFieldName, "GET /cart"),
			expected: TraceQueryParams{OperationName: "GET /cart", Attributes: pcommon.NewMap()},
		},
		{
			name: "the inclusive duration bounds become the duration range",
			filter: compare(expression.OpAnd,
				bound(expression.OpGte, 2*time.Second),
				bound(expression.OpLte, 90*time.Second)),
			expected: TraceQueryParams{
				DurationMin: 2 * time.Second,
				DurationMax: 90 * time.Second,
				Attributes:  pcommon.NewMap(),
			},
		},
		{
			name: "a conjunction fills several fields at once",
			filter: compare(expression.OpAnd,
				text(expression.OpEq, expression.LevelResource, expression.ResourceFieldService, "cart"),
				tag(expression.OpEq, "http.method", "GET"),
				tag(expression.OpEq, "http.status_code", "500")),
			expected: TraceQueryParams{
				ServiceName: "cart",
				Attributes:  attributes(map[string]string{"http.method": "GET", "http.status_code": "500"}),
			},
		},
		{
			// `and` is associative, so a nested conjunction asks the same question as a flat one
			// and is flattened rather than refused.
			name: "a conjunction nested in a conjunction",
			filter: compare(expression.OpAnd,
				text(expression.OpEq, expression.LevelResource, expression.ResourceFieldService, "cart"),
				compare(expression.OpAnd,
					tag(expression.OpEq, "http.method", "GET"),
					compare(expression.OpAnd,
						tag(expression.OpEq, "http.status_code", "500"),
						bound(expression.OpGte, 2*time.Second)))),
			expected: TraceQueryParams{
				ServiceName: "cart",
				DurationMin: 2 * time.Second,
				Attributes:  attributes(map[string]string{"http.method": "GET", "http.status_code": "500"}),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := filterQuery(test.filter).ToLegacyShape()
			require.NoError(t, err)
			assert.Nil(t, got.Filter, "the filter is consumed, so the reader sees one filtering model")
			// Attributes are compared by content, because the map is a set of tag filters and the
			// order the rewrite inserts them in is not part of what a reader is promised.
			assert.Equal(t, test.expected.Attributes.AsRaw(), got.Attributes.AsRaw())
			expected, actual := test.expected, got
			expected.Attributes, actual.Attributes = pcommon.NewMap(), pcommon.NewMap()
			assert.Equal(t, expected, actual)
		})
	}
}

// TestToLegacyShape_RefusesWhatTheFieldsCannotExpress covers the other half of the
// contract: a predicate the legacy fields cannot carry is refused rather than widened into
// a different question.
func TestToLegacyShape_RefusesWhatTheFieldsCannotExpress(t *testing.T) {
	tests := []struct {
		name        string
		filter      *expression.Call
		expectedErr string
	}{
		{
			name: "a disjunction",
			filter: compare(expression.OpOr,
				tag(expression.OpEq, "a", "1"), tag(expression.OpEq, "b", "1")),
			expectedErr: `does not support the operator "or"`,
		},
		{
			name:        "a negation",
			filter:      compare(expression.OpNot, tag(expression.OpEq, "a", "1")),
			expectedErr: `does not support the operator "not"`,
		},
		{
			name: "a disjunction nested in a conjunction",
			filter: compare(expression.OpAnd,
				tag(expression.OpEq, "a", "1"),
				compare(expression.OpOr, tag(expression.OpEq, "b", "1"), tag(expression.OpEq, "c", "1"))),
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "a bare reference among the conjuncts",
			filter: compare(expression.OpAnd,
				tag(expression.OpEq, "a", "1"), &expression.AttributeRef{Key: "b"}),
			expectedErr: "conjunction of predicates only",
		},
		{
			name:        "an inequality on an attribute",
			filter:      tag(expression.OpNe, "http.method", "GET"),
			expectedErr: `does not support the operator "ne" on "http.method"`,
		},
		{
			name:        "an exclusive duration bound",
			filter:      bound(expression.OpGt, 2*time.Second),
			expectedErr: `does not support the operator "gt" on "duration"`,
		},
		{
			name:        "a regex on the operation name",
			filter:      text(expression.OpRegex, expression.LevelSpan, expression.SpanFieldName, "GET .*"),
			expectedErr: `does not support the operator "regex" on "name"`,
		},
		{
			name:        "an inequality on the service",
			filter:      text(expression.OpNe, expression.LevelResource, expression.ResourceFieldService, "cart"),
			expectedErr: `does not support the operator "ne" on "service"`,
		},
		{
			name: "an attribute at a level no backend indexes",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "peer.service", Level: expression.LevelLink},
				&expression.AnyValue{Value: "cart"}),
			expectedErr: `does not index the "link" level`,
		},
		{
			name: "a scope-level attribute",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "lib", Level: expression.LevelScope},
				&expression.AnyValue{Value: "otel"}),
			expectedErr: `does not index the "scope" level`,
		},
		{
			name:        "a built-in field with no legacy equivalent",
			filter:      text(expression.OpEq, expression.LevelSpan, expression.SpanFieldKind, "server"),
			expectedErr: `does not support the built-in field "kind" of the "span" level`,
		},
		{
			name: "set membership",
			filter: compare(expression.OpIn,
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}}),
			expectedErr: `does not support the operator "in" on "http.status_code"`,
		},
		{
			name: "an equality against a list",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.List{Values: []string{"500"}}),
			expectedErr: `compares "http.status_code" against a string constant only`,
		},
		{
			name: "an equality of two references",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelSpan},
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelResource}),
			expectedErr: `compares "enduser.id" against a string constant only`,
		},
		{
			// A tag is a string in the legacy map, so a constant that declares a type the map
			// cannot name asks for a narrower match than it can make.
			name: "a tag compared against an integer",
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "http.status_code"},
				&expression.IntValue{Value: 500}),
			expectedErr: `compares "http.status_code" against a string constant only`,
		},
		{
			name: "the operation name compared against a boolean",
			filter: compare(expression.OpEq,
				&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldName},
				&expression.BoolValue{Value: true}),
			expectedErr: `compares "name" against a string constant only`,
		},
		{
			name: "a duration bound that is not a length of time",
			filter: compare(expression.OpGte,
				&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
				&expression.IntValue{Value: 2}),
			expectedErr: `compares "duration" against a duration such as "2s" only`,
		},
		{
			name: "a quantifier over the events",
			filter: compare(expression.OpSome,
				&expression.NestedRef{Level: expression.LevelEvent},
				tag(expression.OpEq, "exception.type", "IOError")),
			expectedErr: "compares a reference against a constant only",
		},
		{
			name:        "an existence test",
			filter:      compare(expression.OpExists, &expression.AttributeRef{Key: "error"}),
			expectedErr: `does not support the operator "exists"`,
		},
		{
			name: "two predicates on the same attribute",
			filter: compare(expression.OpAnd,
				tag(expression.OpEq, "a", "1"), tag(expression.OpEq, "a", "2")),
			expectedErr: `only one predicate on "a"`,
		},
		{
			name: "two services",
			filter: compare(expression.OpAnd,
				text(expression.OpEq, expression.LevelResource, expression.ResourceFieldService, "cart"),
				text(expression.OpEq, expression.LevelResource, expression.ResourceFieldService, "checkout")),
			expectedErr: `only one predicate on "service"`,
		},
		{
			name: "two operation names",
			filter: compare(expression.OpAnd,
				text(expression.OpEq, expression.LevelSpan, expression.SpanFieldName, "a"),
				text(expression.OpEq, expression.LevelSpan, expression.SpanFieldName, "b")),
			expectedErr: `only one predicate on "name"`,
		},
		{
			name: "two lower duration bounds",
			filter: compare(expression.OpAnd,
				bound(expression.OpGte, time.Second), bound(expression.OpGte, 2*time.Second)),
			expectedErr: `only one predicate on "duration"`,
		},
		{
			name: "two upper duration bounds",
			filter: compare(expression.OpAnd,
				bound(expression.OpLte, time.Second), bound(expression.OpLte, 2*time.Second)),
			expectedErr: `only one predicate on "duration"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := filterQuery(test.filter)
			got, err := query.ToLegacyShape()
			require.ErrorIs(t, err, ErrFilterUnsupported)
			assert.Contains(t, err.Error(), test.expectedErr)
			assert.Equal(t, query, got, "the query is returned unchanged so nothing half-rewritten reaches storage")
		})
	}
}

// TestToLegacyShape_RefusesAnUnresolvedDurationBound covers a bound nothing read as a duration.
// A caller reaching storage has been through expression.ResolveConstants, which is where a
// spelling that is not a duration is refused, so an untyped bound here says the query path
// skipped that rather than that the caller wrote the bound wrong.
func TestToLegacyShape_RefusesAnUnresolvedDurationBound(t *testing.T) {
	filter := compare(expression.OpGte,
		&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
		&expression.AnyValue{Value: "2s"})
	_, err := filterQuery(filter).ToLegacyShape()
	require.ErrorIs(t, err, ErrFilterInvalid)
	assert.Contains(t, err.Error(), `the bound "2s" on "duration" was never read as a duration`)
}

// TestToFilterShape covers the inverse: every scalar predicate becomes a filter predicate, the
// fields it came from are cleared so nothing says the same thing twice, and the envelope is
// left alone.
func TestToFilterShape(t *testing.T) {
	query := TraceQueryParams{
		ServiceName:   "cart",
		OperationName: "GET /cart",
		Attributes:    attributes(map[string]string{"http.status_code": "500"}),
		DurationMin:   2 * time.Second,
		DurationMax:   90 * time.Second,
		SearchDepth:   20,
	}

	got := query.ToFilterShape()

	assert.Empty(t, got.ServiceName)
	assert.Empty(t, got.OperationName)
	assert.Zero(t, got.DurationMin)
	assert.Zero(t, got.DurationMax)
	assert.Equal(t, 0, got.Attributes.Len())
	assert.Equal(t, 20, got.SearchDepth, "the envelope is not a predicate")

	require.NotNil(t, got.Filter)
	assert.Equal(t, expression.OpAnd, got.Filter.Op)
	assert.Equal(t, []expression.Expression{
		text(expression.OpEq, expression.LevelResource, expression.ResourceFieldService, "cart"),
		text(expression.OpEq, expression.LevelSpan, expression.SpanFieldName, "GET /cart"),
		tag(expression.OpEq, "http.status_code", "500"),
		bound(expression.OpGte, 2*time.Second),
		bound(expression.OpLte, 90*time.Second),
	}, got.Filter.Args)
}

// TestToFilterShape_NeedsNoResolution pins that the predicates it writes already carry the type
// their field declares, so the filter an interceptor is shown is one it can read straight off
// rather than one still waiting to be resolved.
func TestToFilterShape_NeedsNoResolution(t *testing.T) {
	converted := TraceQueryParams{
		ServiceName:   "cart",
		OperationName: "GET /cart",
		Attributes:    attributes(map[string]string{"http.status_code": "500"}),
		DurationMin:   2 * time.Second,
	}.ToFilterShape()

	require.NoError(t, ValidateFilter(converted.Filter))
	resolved, err := ResolveFilterConstants(converted.Filter)
	require.NoError(t, err)
	assert.Equal(t, converted.Filter, resolved)
}

// TestToFilterShape_RoundTripsThroughLegacy pins that the two conversions invert each other,
// which is what lets an interceptor be shown filter shape and storage still receive the fields
// it was going to get.
func TestToFilterShape_RoundTripsThroughLegacy(t *testing.T) {
	query := TraceQueryParams{
		ServiceName:   "cart",
		OperationName: "GET /cart",
		Attributes:    attributes(map[string]string{"http.method": "GET"}),
		DurationMin:   2 * time.Second,
		DurationMax:   90 * time.Second,
		SearchDepth:   20,
	}

	back, err := query.ToFilterShape().ToLegacyShape()
	require.NoError(t, err)
	assert.Equal(t, query.Attributes.AsRaw(), back.Attributes.AsRaw())
	expected, actual := query, back
	expected.Attributes, actual.Attributes = pcommon.NewMap(), pcommon.NewMap()
	assert.Equal(t, expected, actual)
}

func TestToFilterShape_LeavesAFilterAlone(t *testing.T) {
	filter := tag(expression.OpEq, "a", "1")
	got := filterQuery(filter).ToFilterShape()
	assert.Equal(t, filter, got.Filter)
}

// TestToFilterShape_NoPredicates pins that a search of the time range alone converts to no
// filter. There is no expression for "match everything" — an `and` of nothing takes fewer
// arguments than `and` accepts — so a nil Filter is what says it.
func TestToFilterShape_NoPredicates(t *testing.T) {
	got := TraceQueryParams{Attributes: pcommon.NewMap()}.ToFilterShape()
	assert.Nil(t, got.Filter)
}

// TestToFilterShape_UninitializedAttributes pins that a query whose Attributes was left at its
// zero value converts rather than panicking; it used to reach storage unharmed.
func TestToFilterShape_UninitializedAttributes(t *testing.T) {
	got := TraceQueryParams{ServiceName: "cart"}.ToFilterShape()
	assert.Equal(t,
		text(expression.OpEq, expression.LevelResource, expression.ResourceFieldService, "cart"),
		got.Filter,
		"one predicate stands on its own rather than in a conjunction of one")
}

func TestToLegacyShape_WithoutAFilterIsUnchanged(t *testing.T) {
	query := TraceQueryParams{ServiceName: "cart", Attributes: pcommon.NewMap()}
	got, err := query.ToLegacyShape()
	require.NoError(t, err)
	assert.Equal(t, query, got)
}
