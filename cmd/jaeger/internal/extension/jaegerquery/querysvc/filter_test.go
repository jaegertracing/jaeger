// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func predicate(op expression.Operator, ref *expression.Reference, value string) *expression.Call {
	return &expression.Call{Op: op, Args: []expression.Expression{ref, &expression.Scalar{Value: value}}}
}

func filterQuery(filter *expression.Call) TraceQueryParams {
	return TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(),
			Filter:     filter,
		},
	}
}

// TestPrepareFilter_PassesFilterToADeclaringReader covers a backend that evaluates the
// filter itself: the filter reaches it untouched and the legacy fields stay empty.
func TestPrepareFilter_PassesFilterToADeclaringReader(t *testing.T) {
	caps := tracestore.SearchCapabilities{Filter: &tracestore.FilterCapabilities{
		Levels: []expression.Level{expression.LevelSpan, expression.LevelResource, expression.LevelEvent},
		Operators: []expression.Operator{
			expression.OpAnd, expression.OpOr,
			expression.OpEq, expression.OpGt, expression.OpRegex, expression.OpSome,
		},
	}}
	filter := &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
		predicate(expression.OpGt, expression.SpanDuration.Ref(), "2s"),
		&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
			predicate(expression.OpEq, &expression.Reference{Name: "http.status_code"}, "500"),
			predicate(expression.OpRegex, &expression.Reference{Name: "http.route", Level: expression.LevelSpan, Attr: true}, "/cart/.*"),
		}},
	}}
	query := filterQuery(filter)

	got, err := prepareFilter(query, caps)
	require.NoError(t, err)
	assert.Equal(t, query, got)
}

// TestPrepareFilter_RefusesWhatAReaderDidNotDeclare covers the refusal gates: a level or an
// operator absent from the declaration is refused before dispatch. The boolean combinators
// ride the same gate, so a reader confined to the conjunctive subset is one that lists OpAnd
// and omits OpOr and OpNot.
func TestPrepareFilter_RefusesWhatAReaderDidNotDeclare(t *testing.T) {
	eqRef := func(name string) *expression.Call {
		return predicate(expression.OpEq, &expression.Reference{Name: name}, "1")
	}
	conjunctive := tracestore.FilterCapabilities{
		Operators: []expression.Operator{expression.OpAnd, expression.OpEq},
	}
	tests := []struct {
		name        string
		caps        tracestore.FilterCapabilities
		filter      *expression.Call
		expectedErr string
	}{
		{
			name:        "an operator it did not list",
			caps:        conjunctive,
			filter:      predicate(expression.OpRegex, &expression.Reference{Name: "a"}, "b.*"),
			expectedErr: `does not support the operator "regex"`,
		},
		{
			name: "an operator it did not list, nested in a conjunction",
			caps: conjunctive,
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eqRef("a"),
				predicate(expression.OpGt, &expression.Reference{Name: "b"}, "1"),
			}},
			expectedErr: `does not support the operator "gt"`,
		},
		{
			name:        "a disjunction against a flat index",
			caps:        conjunctive,
			filter:      &expression.Call{Op: expression.OpOr, Args: []expression.Expression{eqRef("a"), eqRef("b")}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "a disjunction nested in a conjunction against a flat index",
			caps: conjunctive,
			filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				eqRef("a"),
				&expression.Call{Op: expression.OpOr, Args: []expression.Expression{eqRef("b"), eqRef("c")}},
			}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "a level it does not index",
			caps: tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpEq},
			},
			filter: predicate(expression.OpEq,
				&expression.Reference{Name: "peer.service", Level: expression.LevelLink, Attr: true}, "cart"),
			expectedErr: `does not index the "link" level`,
		},
		{
			name: "a level it does not index, on the right of a comparison",
			caps: tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpNe},
			},
			filter: &expression.Call{Op: expression.OpNe, Args: []expression.Expression{
				&expression.Reference{Name: "enduser.id", Level: expression.LevelSpan, Attr: true},
				&expression.Reference{Name: "enduser.id", Level: expression.LevelResource, Attr: true},
			}},
			expectedErr: `does not index the "resource" level`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caps := tracestore.SearchCapabilities{Filter: &test.caps}
			_, err := prepareFilter(filterQuery(test.filter), caps)
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Contains(t, err.Error(), test.expectedErr)
		})
	}
}

// TestPrepareFilter_RefusesLegacyPredicatesAlongsideAFilter pins the mutual exclusion: the
// two ways of asking for a service, an operation, a duration or a tag cannot be mixed.
func TestPrepareFilter_RefusesLegacyPredicatesAlongsideAFilter(t *testing.T) {
	filter := predicate(expression.OpEq, &expression.Reference{Name: "http.method"}, "GET")
	tests := []struct {
		name     string
		mutate   func(*TraceQueryParams)
		expected string
	}{
		{"service name", func(q *TraceQueryParams) { q.ServiceName = "cart" }, "[service_name]"},
		{"operation name", func(q *TraceQueryParams) { q.OperationName = "GET /cart" }, "[operation_name]"},
		{"duration min", func(q *TraceQueryParams) { q.DurationMin = time.Second }, "[duration_min]"},
		{"duration max", func(q *TraceQueryParams) { q.DurationMax = time.Second }, "[duration_max]"},
		{"attributes", func(q *TraceQueryParams) { q.Attributes.PutStr("a", "1") }, "[attributes]"},
		{
			"several at once",
			func(q *TraceQueryParams) {
				q.ServiceName = "cart"
				q.DurationMax = time.Second
			},
			"[service_name duration_max]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := filterQuery(filter)
			test.mutate(&query)
			_, err := prepareFilter(query, tracestore.SearchCapabilities{})
			require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
			assert.Contains(t, err.Error(), test.expected)
		})
	}
}

func TestIsBadRequest(t *testing.T) {
	assert.True(t, IsBadRequest(ErrServiceNameRequired))
	assert.True(t, IsBadRequest(tracestore.ErrFilterUnsupported))
	assert.True(t, IsBadRequest(tracestore.ErrFilterInvalid))
	assert.True(t, IsBadRequest(fmt.Errorf("%w: nested", tracestore.ErrFilterUnsupported)))
	assert.False(t, IsBadRequest(errors.New("storage is down")))
	assert.False(t, IsBadRequest(nil))
}

// TestPrepareFilter_EmptyDeclarationIsNoDeclaration pins that a reader naming nothing reads as
// one that declared nothing: the filter is rewritten into the legacy predicate fields rather
// than refused, because a reader opts in by naming what it serves and there is no
// half-opted-in state to read differently.
func TestPrepareFilter_EmptyDeclarationIsNoDeclaration(t *testing.T) {
	filter := predicate(expression.OpEq, &expression.Reference{Name: "http.method"}, "GET")

	for name, caps := range map[string]tracestore.SearchCapabilities{
		"no declaration":    {},
		"empty declaration": {Filter: &tracestore.FilterCapabilities{}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := prepareFilter(filterQuery(filter), caps)
			require.NoError(t, err)
			assert.Nil(t, got.Filter, "the filter is rewritten, not sent down")
			value, ok := got.Attributes.Get("http.method")
			require.True(t, ok)
			assert.Equal(t, "GET", value.Str())
		})
	}
}
