// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestFindSpansOrdering(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	data := ptrace.NewTraces()
	for _, f := range []struct {
		name            string
		trace, span     byte
		start, duration int64
	}{
		{"a", 2, 3, 1, 4}, {"b", 1, 2, 3, 2}, {"c", 2, 1, 2, 3}, {"d", 1, 1, 3, 1},
	} {
		rs := data.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", f.name)
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetName(f.name)
		span.SetTraceID(pcommon.TraceID{f.trace})
		span.SetSpanID(pcommon.SpanID{f.span})
		span.SetStartTimestamp(pcommon.Timestamp(f.start))
		span.SetEndTimestamp(pcommon.Timestamp(f.start + f.duration))
	}
	require.NoError(t, store.WriteTraces(t.Context(), data))
	for _, tc := range []struct {
		field     string
		asc, desc []string
	}{
		{"startTime", []string{"a", "c", "d", "b"}, []string{"d", "b", "c", "a"}},
		{"duration", []string{"d", "b", "c", "a"}, []string{"a", "c", "b", "d"}},
		{"traceID", []string{"d", "b", "c", "a"}, []string{"c", "a", "d", "b"}},
		{"spanID", []string{"d", "c", "b", "a"}, []string{"a", "b", "d", "c"}},
	} {
		for _, direction := range []tracestore.SortDirection{tracestore.SortAscending, tracestore.SortDescending} {
			t.Run(tc.field+string(direction), func(t *testing.T) {
				query := tracestore.SpanQueryParams{OrderBy: []tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: tc.field}, Direction: direction}}, Pagination: tracestore.Pagination{PageSize: 1}}
				var names []string
				for range 4 {
					chunk, err := findSpansPage(t, store, query)
					require.NoError(t, err)
					names = append(names, spanNames(chunk.Results)...)
					query.Pagination.PageToken = chunk.NextPageToken
				}
				want := tc.asc
				if direction == tracestore.SortDescending {
					want = tc.desc
				}
				assert.Equal(t, want, names)
				assert.Empty(t, query.Pagination.PageToken)
			})
		}
	}
	caps, err := store.SearchCapabilities(t.Context())
	require.NoError(t, err)
	assert.True(t, caps.SpanSorting)
}

func TestSpanOrderingCursorValues(t *testing.T) {
	key := spanKey{startTime: pcommon.Timestamp(42), endTime: pcommon.Timestamp(17), traceID: pcommon.TraceID{0xff}, spanID: pcommon.SpanID{0xfe}}
	assert.Equal(t, key, decodeSpanKey(key.encode()))
	order, err := tracestore.EffectiveSpanOrder([]tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "duration"}}})
	require.NoError(t, err)
	compare := compareSpanKeys(order)
	assert.Negative(t, compare(key, spanKey{}), "a negative duration sorts before zero")
	later := key
	later.endTime = pcommon.NewTimestampFromTime(key.startTime.AsTime().Add(time.Second))
	assert.Positive(t, compare(later, key))
}

func TestFindSpansOrderedDuplicatesAndChangedToken(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	writeTracesStartingAt(t, store, 2, base)
	writeTracesStartingAt(t, store, 2, base)
	query := tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 1}, OrderBy: []tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "traceID"}, Direction: tracestore.SortAscending}}}
	var ids []byte
	var firstToken tracestore.PageToken
	for range 4 {
		chunk, err := findSpansPage(t, store, query)
		require.NoError(t, err)
		require.Equal(t, 1, chunk.Results.SpanCount())
		ids = append(ids, chunk.Results.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()[0])
		if firstToken == "" {
			firstToken = chunk.NextPageToken
		}
		query.Pagination.PageToken = chunk.NextPageToken
	}
	assert.Equal(t, []byte{1, 1, 2, 2}, ids)
	assert.Empty(t, query.Pagination.PageToken)
	query.Pagination.PageToken = firstToken
	query.OrderBy[0].Direction = tracestore.SortDescending
	_, err = findSpansPage(t, store, query)
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	_, _, err = store.getTenant("").findSpans(tracestore.SpanQueryParams{OrderBy: []tracestore.SpanSortOrder{{}}}, nil)
	require.ErrorIs(t, err, tracestore.ErrSpanOrderInvalid)
}
