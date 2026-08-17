// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
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

// TestPrepareFilteredQuery_PassesFilterToADeclaringReader covers a backend that evaluates the
// filter itself: the filter reaches it untouched and the legacy fields stay empty.
func TestPrepareFilteredQuery_PassesFilterToADeclaringReader(t *testing.T) {
	caps := tracestore.SearchCapabilities{Filter: &tracestore.FilterCapabilities{
		Levels: []expression.Level{expression.LevelSpan, expression.LevelResource, expression.LevelEvent},
		Operators: []expression.Operator{
			expression.OpAnd, expression.OpOr,
			expression.OpEq, expression.OpGt, expression.OpRegex, expression.OpSome,
		},
	}}
	filter := &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
		predicate(expression.OpGt, &expression.Reference{Level: expression.LevelSpan, Name: expression.SpanFieldDuration}, "2s"),
		&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
			predicate(expression.OpEq, &expression.Reference{Name: "http.status_code"}, "500"),
			predicate(expression.OpRegex, &expression.Reference{Name: "http.route", Level: expression.LevelSpan, Attr: true}, "/cart/.*"),
		}},
	}}
	query := filterQuery(filter)

	got, err := prepareFilteredQuery(query, caps)
	require.NoError(t, err)
	assert.Equal(t, query, got)
}

// TestPrepareFilteredQuery_RefusesWhatAReaderDidNotDeclare covers the refusal gates: a level or an
// operator absent from the declaration is refused before dispatch. The boolean combinators
// ride the same gate, so a reader confined to the conjunctive subset is one that lists OpAnd
// and omits OpOr and OpNot.
func TestPrepareFilteredQuery_RefusesWhatAReaderDidNotDeclare(t *testing.T) {
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
			_, err := prepareFilteredQuery(filterQuery(test.filter), caps)
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Contains(t, err.Error(), test.expectedErr)
		})
	}
}

// TestPrepareSearchQuery_RefusesLegacyPredicatesAlongsideAFilter pins the mutual exclusion: the
// two ways of asking for a service, an operation, a duration or a tag cannot be mixed. The
// request is malformed whatever the backend can do, so the reader is asked for nothing — it has
// no expectations set, and any call to it would fail the test.
func TestPrepareSearchQuery_RefusesLegacyPredicatesAlongsideAFilter(t *testing.T) {
	enableStructuredFilters(t)
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
			reader := new(tracestoremocks.Reader)
			qs := NewQueryService(reader, nil, QueryServiceOptions{})

			_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), query))
			require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
			assert.Contains(t, err.Error(), test.expected)
			reader.AssertExpectations(t)
		})
	}
}

// enableStructuredFilters turns the filter feature gate on for one test and restores it
// afterwards. The gate is off by default, so a test that dispatches a filter has to ask for it —
// which is the isolation the gate exists to give a deployment that has not opted in.
func enableStructuredFilters(t *testing.T) {
	setStructuredFilters(t, true)
}

func setStructuredFilters(t *testing.T, enabled bool) {
	original := StructuredFiltersGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(StructuredFiltersGate.ID(), enabled))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(StructuredFiltersGate.ID(), original))
	})
}

// TestPrepareSearchQuery_FilterDisabled pins what a deployment that has not opted in does with
// a query carrying a filter: it is refused where every other unserviceable query is refused,
// and the reader is never asked for its capabilities, let alone dispatched to. Alpha is what
// makes that the default, which TestStructuredFiltersGate_IsAlpha pins separately.
func TestPrepareSearchQuery_FilterDisabled(t *testing.T) {
	setStructuredFilters(t, false)

	reader := new(tracestoremocks.Reader)
	qs := NewQueryService(reader, nil, QueryServiceOptions{})
	query := filterQuery(predicate(expression.OpEq, &expression.Reference{Name: "a"}, "1"))

	_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), query))
	require.ErrorIs(t, err, ErrFilterDisabled)
	require.ErrorContains(t, err, "jaeger.query.structuredFilters")
	assert.True(t, IsBadRequest(err), "the API layers answer 400")
	reader.AssertExpectations(t)
}

// TestPrepareSearchQuery_RefusesAMalformedFilter covers the boundary check the conversion no
// longer does: decoding a filter off a wire yields whatever tree the wire described, so the query
// service is where a tree this build has no meaning for is refused — before the reader is asked
// anything, since it has no expectations set here.
func TestPrepareSearchQuery_RefusesAMalformedFilter(t *testing.T) {
	enableStructuredFilters(t)
	reader := new(tracestoremocks.Reader)
	qs := NewQueryService(reader, nil, QueryServiceOptions{})
	query := filterQuery(&expression.Call{Op: "matches", Args: []expression.Expression{
		&expression.Reference{Name: "a"}, &expression.Scalar{Value: "b"},
	}})

	_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), query))
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	require.ErrorContains(t, err, `unknown filter operator "matches"`)
	assert.True(t, IsBadRequest(err), "the API layers answer 400")
	reader.AssertExpectations(t)
}

func TestStructuredFiltersGate_IsAlpha(t *testing.T) {
	assert.Equal(t, featuregate.StageAlpha, StructuredFiltersGate.Stage(),
		"Alpha is what keeps the filter off unless a deployment asks for it")
}

func TestIsBadRequest(t *testing.T) {
	assert.True(t, IsBadRequest(ErrServiceNameRequired))
	assert.True(t, IsBadRequest(tracestore.ErrFilterUnsupported))
	assert.True(t, IsBadRequest(tracestore.ErrFilterInvalid))
	assert.True(t, IsBadRequest(ErrFilterDisabled))
	assert.True(t, IsBadRequest(fmt.Errorf("%w: nested", tracestore.ErrFilterUnsupported)))
	assert.False(t, IsBadRequest(errors.New("storage is down")))
	assert.False(t, IsBadRequest(nil))
}

// TestPrepareFilteredQuery_EmptyDeclarationIsNoDeclaration pins that a reader naming nothing reads as
// one that declared nothing: the filter is rewritten into the legacy predicate fields rather
// than refused, because a reader opts in by naming what it serves and there is no
// half-opted-in state to read differently.
func TestPrepareFilteredQuery_EmptyDeclarationIsNoDeclaration(t *testing.T) {
	filter := predicate(expression.OpEq, &expression.Reference{Name: "http.method"}, "GET")

	for name, caps := range map[string]tracestore.SearchCapabilities{
		"no declaration":    {},
		"empty declaration": {Filter: &tracestore.FilterCapabilities{}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := prepareFilteredQuery(filterQuery(filter), caps)
			require.NoError(t, err)
			assert.Nil(t, got.Filter, "the filter is rewritten, not sent down")
			value, ok := got.Attributes.Get("http.method")
			require.True(t, ok)
			assert.Equal(t, "GET", value.Str())
		})
	}
}
