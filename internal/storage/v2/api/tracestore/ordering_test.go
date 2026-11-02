// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
)

func sortTerm(name string, direction SortDirection) SpanSortOrder {
	return SpanSortOrder{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: name}, Direction: direction}
}

func TestNormalizeSpanOrder(t *testing.T) {
	for _, name := range []string{"startTime", "duration", "traceID", "spanID"} {
		for _, direction := range []SortDirection{"", SortAscending, SortDescending} {
			t.Run(name+string(direction), func(t *testing.T) {
				input := []SpanSortOrder{sortTerm(name, direction)}
				got, err := NormalizeSpanOrder(input)
				require.NoError(t, err)
				want := direction
				if want == "" {
					want = SortAscending
				}
				assert.Equal(t, []SpanSortOrder{sortTerm(name, want)}, got)
				assert.Equal(t, direction, input[0].Direction)
				got[0].Expression.(*expression.FieldRef).Name = "changed"
				assert.Equal(t, name, input[0].Expression.(*expression.FieldRef).Name)
			})
		}
	}
	t.Run("multiple terms", func(t *testing.T) {
		input := []SpanSortOrder{sortTerm("duration", SortDescending), sortTerm("startTime", ""), sortTerm("spanID", SortAscending)}
		got, err := NormalizeSpanOrder(input)
		require.NoError(t, err)
		assert.Equal(t, []SpanSortOrder{sortTerm("duration", SortDescending), sortTerm("startTime", SortAscending), sortTerm("spanID", SortAscending)}, got)
		assert.Equal(t, SortDirection(""), input[1].Direction)
	})
	got, err := NormalizeSpanOrder(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
	for name, order := range map[string][]SpanSortOrder{
		"missing": {{}}, "typed nil": {{Expression: (*expression.FieldRef)(nil)}},
		"attribute":     {{Expression: &expression.AttributeRef{Key: "duration"}}},
		"constant":      {{Expression: &expression.AnyValue{Value: "duration"}}},
		"call":          {{Expression: &expression.Call{Op: expression.OpEq}}},
		"nested":        {{Expression: &expression.NestedRef{Level: expression.LevelEvent}}},
		"other level":   {{Expression: &expression.FieldRef{Level: expression.LevelResource, Name: "startTime"}}},
		"unqualified":   {{Expression: &expression.FieldRef{Name: "startTime"}}},
		"unknown field": {sortTerm("name", "")},
		"direction":     {sortTerm("duration", "ASC")},
		"duplicate":     {sortTerm("duration", "asc"), sortTerm("duration", "desc")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeSpanOrder(order)
			require.ErrorIs(t, err, ErrSpanOrderInvalid)
		})
	}
}

func TestEffectiveSpanOrder(t *testing.T) {
	defaults := []SpanSortOrder{sortTerm("startTime", "desc"), sortTerm("traceID", "asc"), sortTerm("spanID", "asc")}
	for _, order := range [][]SpanSortOrder{nil, defaults, {sortTerm("startTime", "desc")}} {
		got, err := EffectiveSpanOrder(order)
		require.NoError(t, err)
		assert.Equal(t, defaults, got)
	}
	duration := []SpanSortOrder{sortTerm("duration", "desc")}
	effective, err := EffectiveSpanOrder(duration)
	require.NoError(t, err)
	assert.Equal(t, append(duration, defaults...), effective)

	// The result is a fixed point, so a backend may settle an order the query service already settled.
	again, err := EffectiveSpanOrder(effective)
	require.NoError(t, err)
	assert.Equal(t, effective, again)

	// An omitted direction is defaulted before the tie-breakers are chosen.
	settled, err := EffectiveSpanOrder([]SpanSortOrder{sortTerm("traceID", "")})
	require.NoError(t, err)
	assert.Equal(t, []SpanSortOrder{sortTerm("traceID", "asc"), sortTerm("startTime", "desc"), sortTerm("spanID", "asc")}, settled)

	_, err = EffectiveSpanOrder([]SpanSortOrder{sortTerm("name", "asc")})
	require.ErrorIs(t, err, ErrSpanOrderInvalid)
}

func TestSpanOrderFingerprint(t *testing.T) {
	orders := [][]SpanSortOrder{
		nil,
		{sortTerm("startTime", "asc")},
		{sortTerm("duration", "desc")},
		{sortTerm("duration", "asc")},
		{sortTerm("traceID", "asc"), sortTerm("duration", "desc")},
		{sortTerm("duration", "desc"), sortTerm("traceID", "asc")},
	}
	seen := map[string]bool{}
	for _, order := range orders {
		query := SpanQueryParams{OrderBy: order}
		fp, err := query.Fingerprint()
		require.NoError(t, err)
		assert.False(t, seen[string(fp)])
		seen[string(fp)] = true
		query.Pagination.PageSize = 42
		same, err := query.Fingerprint()
		require.NoError(t, err)
		assert.Equal(t, fp, same)
	}
}

func TestSpanSortingCapability(t *testing.T) {
	for _, caps := range []SearchCapabilities{{}, {SpanSearch: true}, {SpanSorting: true}, {SpanSearch: true, SpanSorting: true}} {
		require.NoError(t, caps.ValidateSpanSorting(nil))
		err := caps.ValidateSpanSorting([]SpanSortOrder{sortTerm("startTime", "desc")})
		if caps.SpanSearch && caps.SpanSorting {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, ErrSpanOrderUnsupported)
			require.NotErrorIs(t, err, ErrSpanOrderInvalid)
		}
	}
}

func TestSpanOrderProto(t *testing.T) {
	got, err := SpanOrderFromProto([]*api_v3.SpanSortOrder(nil))
	require.NoError(t, err)
	assert.Nil(t, got)
	for _, term := range []SpanSortOrder{
		sortTerm("duration", ""),
		sortTerm("unknown", "custom-direction"),
		{Expression: &expression.AttributeRef{Level: expression.LevelSpan, Key: "priority"}},
		{Expression: &expression.NestedRef{Level: expression.LevelEvent}},
		{Expression: &expression.IntValue{Value: 42}},
		{Expression: &expression.Call{Op: "custom", Args: []expression.Expression{&expression.StringValue{Value: "a"}}}},
	} {
		encoded, err := expressionproto.ToProto(term.Expression)
		require.NoError(t, err)
		wire := &api_v3.SpanSortOrder{Expression: encoded, Direction: string(term.Direction)}
		got, err = SpanOrderFromProto([]*api_v3.SpanSortOrder{wire, wire})
		require.NoError(t, err)
		assert.Equal(t, []SpanSortOrder{term, term}, got)
	}
	for _, wire := range []*api_v3.SpanSortOrder{nil, {}, {Expression: &expressionproto.Expression{}}} {
		_, err := SpanOrderFromProto([]*api_v3.SpanSortOrder{wire})
		require.ErrorContains(t, err, "cannot decode order_by[0]")
	}
}

func TestSpanOrderBindsContinuationToken(t *testing.T) {
	query := SpanQueryParams{OrderBy: []SpanSortOrder{sortTerm("duration", "desc")}}
	fingerprint, err := query.Fingerprint()
	require.NoError(t, err)
	token, err := NewPageToken(fingerprint, []byte("cursor"))
	require.NoError(t, err)
	query.Pagination.PageSize = 10
	fingerprint, err = query.Fingerprint()
	require.NoError(t, err)
	cursor, err := token.Cursor(fingerprint)
	require.NoError(t, err)
	assert.Equal(t, []byte("cursor"), cursor)
	query.OrderBy[0].Direction = SortAscending
	fingerprint, err = query.Fingerprint()
	require.NoError(t, err)
	_, err = token.Cursor(fingerprint)
	require.ErrorIs(t, err, ErrPaginationInvalid)
}
