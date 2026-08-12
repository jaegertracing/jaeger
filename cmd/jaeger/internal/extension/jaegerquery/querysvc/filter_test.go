// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func predicate(op tracestore.Operator, ref *tracestore.Reference, value string) *tracestore.Call {
	return &tracestore.Call{Op: op, Args: []tracestore.Expression{ref, &tracestore.Scalar{Value: value}}}
}

func filterQuery(filter *tracestore.Call) TraceQueryParams {
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
		Levels: []tracestore.Level{tracestore.LevelSpan, tracestore.LevelResource, tracestore.LevelEvent},
		Operators: []tracestore.Operator{
			tracestore.OpAnd, tracestore.OpOr,
			tracestore.OpEq, tracestore.OpGt, tracestore.OpRegex, tracestore.OpSome,
		},
	}}
	filter := &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
		predicate(tracestore.OpGt, tracestore.SpanDuration.Ref(), "2s"),
		&tracestore.Call{Op: tracestore.OpOr, Args: []tracestore.Expression{
			predicate(tracestore.OpEq, &tracestore.Reference{Name: "http.status_code"}, "500"),
			predicate(tracestore.OpRegex, &tracestore.Reference{Name: "http.route", Level: tracestore.LevelSpan, Attr: true}, "/cart/.*"),
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
	eqRef := func(name string) *tracestore.Call {
		return predicate(tracestore.OpEq, &tracestore.Reference{Name: name}, "1")
	}
	conjunctive := tracestore.FilterCapabilities{
		Operators: []tracestore.Operator{tracestore.OpAnd, tracestore.OpEq},
	}
	tests := []struct {
		name        string
		caps        tracestore.FilterCapabilities
		filter      *tracestore.Call
		expectedErr string
	}{
		{
			name:        "an operator it did not list",
			caps:        conjunctive,
			filter:      predicate(tracestore.OpRegex, &tracestore.Reference{Name: "a"}, "b.*"),
			expectedErr: `does not support the operator "regex"`,
		},
		{
			name: "an operator it did not list, nested in a conjunction",
			caps: conjunctive,
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				eqRef("a"),
				predicate(tracestore.OpGt, &tracestore.Reference{Name: "b"}, "1"),
			}},
			expectedErr: `does not support the operator "gt"`,
		},
		{
			name:        "a disjunction against a flat index",
			caps:        conjunctive,
			filter:      &tracestore.Call{Op: tracestore.OpOr, Args: []tracestore.Expression{eqRef("a"), eqRef("b")}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "a disjunction nested in a conjunction against a flat index",
			caps: conjunctive,
			filter: &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{
				eqRef("a"),
				&tracestore.Call{Op: tracestore.OpOr, Args: []tracestore.Expression{eqRef("b"), eqRef("c")}},
			}},
			expectedErr: `does not support the operator "or"`,
		},
		{
			name:        "a conjunction against a reader that declares no operator at all",
			caps:        tracestore.FilterCapabilities{},
			filter:      &tracestore.Call{Op: tracestore.OpAnd, Args: []tracestore.Expression{eqRef("a"), eqRef("b")}},
			expectedErr: `does not support the operator "and"`,
		},
		{
			name: "a level it does not index",
			caps: tracestore.FilterCapabilities{
				Levels:    []tracestore.Level{tracestore.LevelSpan},
				Operators: []tracestore.Operator{tracestore.OpEq},
			},
			filter: predicate(tracestore.OpEq,
				&tracestore.Reference{Name: "peer.service", Level: tracestore.LevelLink, Attr: true}, "cart"),
			expectedErr: `does not index the "link" level`,
		},
		{
			name: "a level it does not index, on the right of a comparison",
			caps: tracestore.FilterCapabilities{
				Levels:    []tracestore.Level{tracestore.LevelSpan},
				Operators: []tracestore.Operator{tracestore.OpNe},
			},
			filter: &tracestore.Call{Op: tracestore.OpNe, Args: []tracestore.Expression{
				&tracestore.Reference{Name: "enduser.id", Level: tracestore.LevelSpan, Attr: true},
				&tracestore.Reference{Name: "enduser.id", Level: tracestore.LevelResource, Attr: true},
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
	filter := predicate(tracestore.OpEq, &tracestore.Reference{Name: "http.method"}, "GET")
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
	assert.True(t, IsBadRequest(errUnsupportedOperator(tracestore.OpOr)))
	// Raised below the query service, by the reader an interceptor decorates, and still the
	// caller's problem rather than a server fault.
	assert.True(t, IsBadRequest(queryinterceptor.ErrFilterNotInterceptable))
	assert.False(t, IsBadRequest(errors.New("storage is down")))
	assert.False(t, IsBadRequest(nil))
}
