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
			_, err = EffectiveSpanOrder(order)
			require.ErrorIs(t, err, ErrSpanOrderInvalid)
			_, err = (SpanQueryParams{OrderBy: order}).Fingerprint()
			require.ErrorIs(t, err, ErrSpanOrderInvalid)
		})
	}
}

func TestEffectiveSpanOrderAndFingerprint(t *testing.T) {
	defaults := []SpanSortOrder{sortTerm("startTime", "desc"), sortTerm("traceID", "asc"), sortTerm("spanID", "asc")}
	for _, order := range [][]SpanSortOrder{nil, defaults, {sortTerm("startTime", "desc")}} {
		got, err := EffectiveSpanOrder(order)
		require.NoError(t, err)
		assert.Equal(t, defaults, got)
		fp, err := (SpanQueryParams{OrderBy: order}).Fingerprint()
		require.NoError(t, err)
		want, err := (SpanQueryParams{}).Fingerprint()
		require.NoError(t, err)
		assert.Equal(t, want, fp)
	}
	duration := []SpanSortOrder{sortTerm("duration", "desc")}
	effective, err := EffectiveSpanOrder(duration)
	require.NoError(t, err)
	assert.Equal(t, append(duration, defaults...), effective)
	orders := [][]SpanSortOrder{
		nil,
		{sortTerm("startTime", "asc")},
		duration,
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
		effective, err := EffectiveSpanOrder(order)
		require.NoError(t, err)
		normalized, err := (SpanQueryParams{OrderBy: effective}).Fingerprint()
		require.NoError(t, err)
		assert.Equal(t, fp, normalized)
	}
}

func TestSpanSortingCapability(t *testing.T) {
	for _, caps := range []SearchCapabilities{{}, {SpanSearch: true}, {SpanSorting: true}, {SpanSearch: true, SpanSorting: true}} {
		require.NoError(t, caps.ValidateSpanSorting(nil))
		err := caps.ValidateSpanSorting([]SpanSortOrder{sortTerm("startTime", "desc")})
		if caps.SpanSearch && caps.SpanSorting {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, ErrSpanOrderInvalid)
		}
	}
}

func TestSpanOrderProto(t *testing.T) {
	got, err := SpanOrderFromProto([]*api_v3.SpanSortOrder(nil))
	require.NoError(t, err)
	assert.Nil(t, got)
	term := sortTerm("duration", "asc")
	got, err = SpanOrderFromProto([]*api_v3.SpanSortOrder{{Expression: SpanOrderExpression(term)}})
	require.NoError(t, err)
	assert.Equal(t, []SpanSortOrder{term}, got)
	for _, wire := range []*api_v3.SpanSortOrder{nil, {}, {Expression: &expressionproto.Expression{}}, {Expression: SpanOrderExpression(term), Direction: "bad"}} {
		_, err := SpanOrderFromProto([]*api_v3.SpanSortOrder{wire})
		require.ErrorIs(t, err, ErrSpanOrderInvalid)
	}
}

func TestSpanOrderBindsContinuationToken(t *testing.T) {
	query := SpanQueryParams{OrderBy: []SpanSortOrder{sortTerm("duration", "desc")}}
	fingerprint, err := query.Fingerprint()
	require.NoError(t, err)
	token, err := NewPageToken(fingerprint, []byte("cursor"))
	require.NoError(t, err)
	effective, err := EffectiveSpanOrder(query.OrderBy)
	require.NoError(t, err)
	query.OrderBy = effective
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
