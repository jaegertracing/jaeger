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

func predicate(op expression.Operator, ref *expression.Reference, value string) *expression.Call {
	return &expression.Call{Op: op, Args: []expression.Expression{ref, &expression.Scalar{Value: value}}}
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
			filter:   predicate(expression.OpEq, &expression.Reference{Name: "http.status_code"}, "500"),
			expected: TraceQueryParams{Attributes: attributes(map[string]string{"http.status_code": "500"})},
		},
		{
			name: "a level-qualified attribute widens to an unqualified tag",
			filter: predicate(expression.OpEq,
				&expression.Reference{Name: "k8s.pod.name", Level: expression.LevelResource, Attr: true}, "cart-0"),
			expected: TraceQueryParams{Attributes: attributes(map[string]string{"k8s.pod.name": "cart-0"})},
		},
		{
			name: "an event-level attribute widens too",
			filter: predicate(expression.OpEq,
				&expression.Reference{Name: "exception.type", Level: expression.LevelEvent, Attr: true}, "IOError"),
			expected: TraceQueryParams{Attributes: attributes(map[string]string{"exception.type": "IOError"})},
		},
		{
			name: "the service built-in becomes the service name",
			filter: predicate(expression.OpEq,
				expression.ResourceService.Ref(), "cart"),
			expected: TraceQueryParams{ServiceName: "cart", Attributes: pcommon.NewMap()},
		},
		{
			name: "the span name built-in becomes the operation name",
			filter: predicate(expression.OpEq,
				expression.SpanName.Ref(), "GET /cart"),
			expected: TraceQueryParams{OperationName: "GET /cart", Attributes: pcommon.NewMap()},
		},
		{
			name: "the inclusive duration bounds become the duration range",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpGte, expression.SpanDuration.Ref(), "2s"),
				predicate(expression.OpLte, expression.SpanDuration.Ref(), "1m30s"),
			}},
			expected: TraceQueryParams{
				DurationMin: 2 * time.Second,
				DurationMax: 90 * time.Second,
				Attributes:  pcommon.NewMap(),
			},
		},
		{
			name: "a conjunction fills several fields at once",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpEq, expression.ResourceService.Ref(), "cart"),
				predicate(expression.OpEq, &expression.Reference{Name: "http.method"}, "GET"),
				predicate(expression.OpEq, &expression.Reference{Name: "http.status_code"}, "500"),
			}},
			expected: TraceQueryParams{
				ServiceName: "cart",
				Attributes:  attributes(map[string]string{"http.method": "GET", "http.status_code": "500"}),
			},
		},
		{
			// `and` is associative, so a nested conjunction asks the same question as a flat one
			// and is flattened rather than refused.
			name: "a conjunction nested in a conjunction",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpEq, expression.ResourceService.Ref(), "cart"),
				&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
					predicate(expression.OpEq, &expression.Reference{Name: "http.method"}, "GET"),
					&expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
						predicate(expression.OpEq, &expression.Reference{Name: "http.status_code"}, "500"),
						predicate(expression.OpGte, expression.SpanDuration.Ref(), "2s"),
					}},
				}},
			}},
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
	eqRef := func(name string) *expression.Call {
		return predicate(expression.OpEq, &expression.Reference{Name: name}, "1")
	}
	tests := []struct {
		name        string
		filter      *expression.Call
		expectedErr string
	}{
		{
			name: "a disjunction",
			filter: &expression.Call{Op: expression.OpOr, Args: []expression.Expression{
				eqRef("a"), eqRef("b"),
			}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name:        "a negation",
			filter:      &expression.Call{Op: expression.OpNot, Args: []expression.Expression{eqRef("a")}},
			expectedErr: `does not support the operator "not"`,
		},
		{
			name: "a disjunction nested in a conjunction",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eqRef("a"),
				&expression.Call{Op: expression.OpOr, Args: []expression.Expression{eqRef("b"), eqRef("c")}},
			}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "a bare reference among the conjuncts",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eqRef("a"), &expression.Reference{Name: "b"},
			}},
			expectedErr: "conjunction of predicates only",
		},
		{
			name:        "an inequality on an attribute",
			filter:      predicate(expression.OpNe, &expression.Reference{Name: "http.method"}, "GET"),
			expectedErr: `does not support the operator "ne" on "http.method"`,
		},
		{
			name: "an exclusive duration bound",
			filter: predicate(expression.OpGt,
				expression.SpanDuration.Ref(), "2s"),
			expectedErr: `does not support the operator "gt" on "duration"`,
		},
		{
			name: "a regex on the operation name",
			filter: predicate(expression.OpRegex,
				expression.SpanName.Ref(), "GET .*"),
			expectedErr: `does not support the operator "regex" on "name"`,
		},
		{
			name: "an inequality on the service",
			filter: predicate(expression.OpNe,
				expression.ResourceService.Ref(), "cart"),
			expectedErr: `does not support the operator "ne" on "service"`,
		},
		{
			name: "an attribute at a level no backend indexes",
			filter: predicate(expression.OpEq,
				&expression.Reference{Name: "peer.service", Level: expression.LevelLink, Attr: true}, "cart"),
			expectedErr: `does not index the "link" level`,
		},
		{
			name: "an instrumentation-level attribute",
			filter: predicate(expression.OpEq,
				&expression.Reference{Name: "lib", Level: expression.LevelInstrumentation, Attr: true}, "otel"),
			expectedErr: `does not index the "instrumentation" level`,
		},
		{
			name: "a built-in field with no legacy equivalent",
			filter: predicate(expression.OpEq,
				&expression.Reference{Name: "kind", Level: expression.LevelSpan}, "server"),
			expectedErr: `does not support the built-in field "kind" of the "span" level`,
		},
		{
			name: "set membership",
			filter: &expression.Call{Op: expression.OpIn, Args: []expression.Expression{
				&expression.Reference{Name: "http.status_code"},
				&expression.List{Values: []string{"500", "503"}},
			}},
			expectedErr: "compares a reference against a constant only",
		},
		{
			name: "a comparison of two references",
			filter: &expression.Call{Op: expression.OpNe, Args: []expression.Expression{
				&expression.Reference{Name: "enduser.id", Level: expression.LevelSpan, Attr: true},
				&expression.Reference{Name: "enduser.id", Level: expression.LevelResource, Attr: true},
			}},
			expectedErr: "compares a reference against a constant only",
		},
		{
			name: "an existence test",
			filter: &expression.Call{Op: expression.OpExists, Args: []expression.Expression{
				&expression.Reference{Name: "error"},
			}},
			expectedErr: `does not support the operator "exists"`,
		},
		{
			name: "two predicates on the same attribute",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpEq, &expression.Reference{Name: "a"}, "1"),
				predicate(expression.OpEq, &expression.Reference{Name: "a"}, "2"),
			}},
			expectedErr: `only one predicate on "a"`,
		},
		{
			name: "two services",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpEq, expression.ResourceService.Ref(), "cart"),
				predicate(expression.OpEq, expression.ResourceService.Ref(), "checkout"),
			}},
			expectedErr: `only one predicate on "service"`,
		},
		{
			name: "two operation names",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpEq, expression.SpanName.Ref(), "a"),
				predicate(expression.OpEq, expression.SpanName.Ref(), "b"),
			}},
			expectedErr: `only one predicate on "name"`,
		},
		{
			name: "two lower duration bounds",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpGte, expression.SpanDuration.Ref(), "1s"),
				predicate(expression.OpGte, expression.SpanDuration.Ref(), "2s"),
			}},
			expectedErr: `only one predicate on "duration"`,
		},
		{
			name: "two upper duration bounds",
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				predicate(expression.OpLte, expression.SpanDuration.Ref(), "1s"),
				predicate(expression.OpLte, expression.SpanDuration.Ref(), "2s"),
			}},
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

// TestToLegacyShape_RefusesAnUnparsableDuration covers the one mistake structural
// validation cannot catch, because the filter AST does not carry types.
func TestToLegacyShape_RefusesAnUnparsableDuration(t *testing.T) {
	filter := predicate(expression.OpGte,
		expression.SpanDuration.Ref(), "quickly")
	_, err := filterQuery(filter).ToLegacyShape()
	require.ErrorIs(t, err, ErrFilterInvalid)
	assert.Contains(t, err.Error(), `"quickly" is not a duration`)
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
		predicate(expression.OpEq, expression.ResourceService.Ref(), "cart"),
		predicate(expression.OpEq, expression.SpanName.Ref(), "GET /cart"),
		predicate(expression.OpEq, &expression.Reference{Name: "http.status_code"}, "500"),
		predicate(expression.OpGte, expression.SpanDuration.Ref(), "2s"),
		predicate(expression.OpLte, expression.SpanDuration.Ref(), "1m30s"),
	}, got.Filter.Args)
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
	filter := predicate(expression.OpEq, &expression.Reference{Name: "a"}, "1")
	got := filterQuery(filter).ToFilterShape()
	assert.Equal(t, filter, got.Filter)
}

// TestToFilterShape_NoPredicates pins the empty conjunction: a caller that understands only
// filters never has to tell "no predicates" from "not converted".
func TestToFilterShape_NoPredicates(t *testing.T) {
	got := TraceQueryParams{Attributes: pcommon.NewMap()}.ToFilterShape()
	require.NotNil(t, got.Filter)
	assert.Equal(t, expression.OpAnd, got.Filter.Op)
	assert.Empty(t, got.Filter.Args)
}

// TestToFilterShape_UninitializedAttributes pins that a query whose Attributes was left at its
// zero value converts rather than panicking; it used to reach storage unharmed.
func TestToFilterShape_UninitializedAttributes(t *testing.T) {
	got := TraceQueryParams{ServiceName: "cart"}.ToFilterShape()
	require.NotNil(t, got.Filter)
	assert.Equal(t, []expression.Expression{
		predicate(expression.OpEq, expression.ResourceService.Ref(), "cart"),
	}, got.Filter.Args)
}

func TestToLegacyShape_WithoutAFilterIsUnchanged(t *testing.T) {
	query := TraceQueryParams{ServiceName: "cart", Attributes: pcommon.NewMap()}
	got, err := query.ToLegacyShape()
	require.NoError(t, err)
	assert.Equal(t, query, got)
}
