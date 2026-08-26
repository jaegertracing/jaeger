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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func compare(op expression.Operator, args ...expression.Expression) *expression.Call {
	return &expression.Call{Op: op, Args: args}
}

// tag builds a predicate on an unqualified attribute, whose constant declares no type because a
// tag never did.
func tag(op expression.Operator, key string, value string) *expression.Call {
	return compare(op, &expression.AttributeRef{Key: key}, &expression.AnyValue{Value: value})
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
	filter := compare(expression.OpAnd,
		compare(expression.OpGt,
			&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
			&expression.DurationValue{Value: 2 * time.Second}),
		compare(expression.OpOr,
			tag(expression.OpEq, "http.status_code", "500"),
			compare(expression.OpRegex,
				&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
				&expression.AnyValue{Value: "/cart/.*"})),
		compare(expression.OpSome,
			&expression.NestedRef{Level: expression.LevelEvent},
			compare(expression.OpEq,
				&expression.FieldRef{Level: expression.LevelEvent, Name: expression.EventFieldName},
				&expression.StringValue{Value: "exception"})))
	query := filterQuery(filter)

	got, err := query.ForCapabilities(caps)
	require.NoError(t, err)
	assert.Equal(t, query.TraceQueryParams, got)
}

// TestPrepareFilteredQuery_RefusesWhatAReaderDidNotDeclare covers the refusal gates: a level or an
// operator absent from the declaration is refused before dispatch. The boolean combinators
// ride the same gate, so a reader confined to the conjunctive subset is one that lists OpAnd
// and omits OpOr and OpNot.
func TestPrepareFilteredQuery_RefusesWhatAReaderDidNotDeclare(t *testing.T) {
	eqRef := func(name string) *expression.Call {
		return tag(expression.OpEq, name, "1")
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
			filter:      tag(expression.OpRegex, "a", "b.*"),
			expectedErr: `does not support the operator "regex"`,
		},
		{
			name:        "an operator it did not list, nested in a conjunction",
			caps:        conjunctive,
			filter:      compare(expression.OpAnd, eqRef("a"), tag(expression.OpGt, "b", "1")),
			expectedErr: `does not support the operator "gt"`,
		},
		{
			name:        "a disjunction against a flat index",
			caps:        conjunctive,
			filter:      compare(expression.OpOr, eqRef("a"), eqRef("b")),
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "a disjunction nested in a conjunction against a flat index",
			caps: conjunctive,
			filter: compare(expression.OpAnd,
				eqRef("a"),
				compare(expression.OpOr, eqRef("b"), eqRef("c"))),
			expectedErr: `does not support the operator "or"`,
		},
		{
			name: "an attribute at a level it does not index",
			caps: tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpEq},
			},
			filter: compare(expression.OpEq,
				&expression.AttributeRef{Key: "peer.service", Level: expression.LevelLink},
				&expression.AnyValue{Value: "cart"}),
			expectedErr: `does not index the "link" level`,
		},
		{
			name: "a built-in field at a level it does not index",
			caps: tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpEq},
			},
			filter: compare(expression.OpEq,
				&expression.FieldRef{Level: expression.LevelScope, Name: expression.ScopeFieldName},
				&expression.StringValue{Value: "otelhttp"}),
			expectedErr: `does not index the "scope" level`,
		},
		{
			name: "a collection it does not index",
			caps: tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpSome, expression.OpEq},
			},
			filter: compare(expression.OpSome,
				&expression.NestedRef{Level: expression.LevelEvent},
				compare(expression.OpEq,
					&expression.FieldRef{Level: expression.LevelEvent, Name: expression.EventFieldName},
					&expression.StringValue{Value: "exception"})),
			expectedErr: `does not index the "event" level`,
		},
		{
			name: "a level it does not index, on the right of a comparison",
			caps: tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan},
				Operators: []expression.Operator{expression.OpNe},
			},
			filter: compare(expression.OpNe,
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelSpan},
				&expression.AttributeRef{Key: "enduser.id", Level: expression.LevelResource}),
			expectedErr: `does not index the "resource" level`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caps := tracestore.SearchCapabilities{Filter: &test.caps}
			_, err := filterQuery(test.filter).ForCapabilities(caps)
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
	filter := tag(expression.OpEq, "http.method", "GET")
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
	query := filterQuery(tag(expression.OpEq, "a", "1"))

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
	query := filterQuery(tag("matches", "a", "b"))

	_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), query))
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	require.ErrorContains(t, err, `unknown filter operator "matches"`)
	assert.True(t, IsBadRequest(err), "the API layers answer 400")
	reader.AssertExpectations(t)
}

// TestPrepareSearchQuery_RefusesAConstantThatDoesNotFitItsField covers the resolution stage: a
// constant compared against a built-in field is read as that field's type here, so a duration
// nobody can read is answered at the query boundary rather than handed to a backend to
// interpret. The reader has no expectations set, so a call to it would fail the test.
func TestPrepareSearchQuery_RefusesAConstantThatDoesNotFitItsField(t *testing.T) {
	enableStructuredFilters(t)
	tests := []struct {
		name        string
		filter      *expression.Call
		expectedErr string
	}{
		{
			name: "a duration nobody can read",
			filter: compare(expression.OpGte,
				&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
				&expression.AnyValue{Value: "quickly"}),
			expectedErr: `cannot compare span.duration against "quickly"`,
		},
		{
			name: "a span kind that is not one of the words",
			filter: compare(expression.OpEq,
				&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldKind},
				&expression.AnyValue{Value: "SPAN_KIND_SERVER"}),
			expectedErr: "not one of unspecified, internal, server, client, producer, consumer",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := new(tracestoremocks.Reader)
			qs := NewQueryService(reader, nil, QueryServiceOptions{})

			_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), filterQuery(test.filter)))
			require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
			require.ErrorContains(t, err, test.expectedErr)
			assert.True(t, IsBadRequest(err), "the API layers answer 400")
			reader.AssertExpectations(t)
		})
	}
}

// TestPrepareSearchQuery_ResolvesADurationBeforeDispatch pins the resolution stage against the
// rewrite that depends on it: a caller writes a duration as a spelling, and the bound reaches a
// reader that serves only the legacy fields as the length of time it names.
func TestPrepareSearchQuery_ResolvesADurationBeforeDispatch(t *testing.T) {
	enableStructuredFilters(t)
	filter := compare(expression.OpGte,
		&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
		&expression.AnyValue{Value: "1m30s"})

	var dispatched tracestore.TraceQueryParams
	reader := forwardsOneTrace(new(tracestoremocks.Reader), &dispatched)
	reader.On("SearchCapabilities", mock.Anything).
		Return(tracestore.SearchCapabilities{WithoutServiceName: true}, nil)

	qs := NewQueryService(reader, nil, QueryServiceOptions{})
	_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), filterQuery(filter)))
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, dispatched.DurationMin)
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
	filter := tag(expression.OpEq, "http.method", "GET")

	for name, caps := range map[string]tracestore.SearchCapabilities{
		"no declaration":    {},
		"empty declaration": {Filter: &tracestore.FilterCapabilities{}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := filterQuery(filter).ForCapabilities(caps)
			require.NoError(t, err)
			assert.Nil(t, got.Filter, "the filter is rewritten, not sent down")
			value, ok := got.Attributes.Get("http.method")
			require.True(t, ok)
			assert.Equal(t, "GET", value.Str())
		})
	}
}
