// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"bytes"
	"math"
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
	key := sortingKey{[]byte("priority"), {0xff}, {}, []byte("region"), {0, 1, 2}}
	original := cursor[sortingKey]{key: key, seen: 3}
	decoded, err := decodeSpanCursor(original.encode(), len(key))
	require.NoError(t, err)
	assert.Equal(t, original, decoded)

	order := []tracestore.SpanSortOrder{
		{Expression: &expression.AttributeRef{Level: expression.LevelSpan, Key: "priority"}, Direction: tracestore.SortAscending},
		{Expression: &expression.AttributeRef{Level: expression.LevelResource, Key: "region"}, Direction: tracestore.SortDescending},
	}
	compare := compareSortingKeys(order)
	assert.Negative(t, compare(sortingKey{[]byte("a"), []byte("a")}, sortingKey{[]byte("b"), []byte("z")}))
	assert.Positive(t, compare(sortingKey{[]byte("a"), []byte("a")}, sortingKey{[]byte("a"), []byte("z")}))
	assert.Zero(t, compare(sortingKey{[]byte("a"), []byte("z")}, sortingKey{[]byte("a"), []byte("z")}))
}

func TestSpanOrderingDuration(t *testing.T) {
	order := []tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "duration"}, Direction: tracestore.SortAscending}}
	span := ptrace.NewSpan()
	span.SetStartTimestamp(42)
	span.SetEndTimestamp(17)
	negative := makeSortingKey(filterCtx{span: span}, order)
	zero := makeSortingKey(filterCtx{span: ptrace.NewSpan()}, order)
	span.SetEndTimestamp(43)
	positive := makeSortingKey(filterCtx{span: span}, order)
	compare := compareSortingKeys(order)
	assert.Negative(t, compare(negative, zero))
	assert.Positive(t, compare(positive, zero))
}

func TestDecodeSpanCursorMalformed(t *testing.T) {
	for _, raw := range [][]byte{
		nil,
		{0x80},
		{5, 1, 2},
		{1, 2},
		{1, 2, 0, 0, 0, 1, 0},
	} {
		_, err := decodeSpanCursor(raw, 1)
		require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	}
	encoded := (cursor[sortingKey]{key: sortingKey{{1}, {2}}, seen: 1}).encode()
	for _, terms := range []int{1, 3} {
		_, err := decodeSpanCursor(encoded, terms)
		require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	}
}

func TestFindSpansOrderedDuplicatesAndChangedToken(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	writeTracesStartingAt(t, store, 2, base)
	writeTracesStartingAt(t, store, 2, base)
	query := tracestore.SpanQueryParams{OrderBy: []tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "traceID"}, Direction: tracestore.SortAscending}}, Pagination: tracestore.Pagination{PageSize: 1}}
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
	_, err = findSpansPage(t, store, tracestore.SpanQueryParams{OrderBy: []tracestore.SpanSortOrder{{}}})
	require.ErrorIs(t, err, tracestore.ErrSpanOrderInvalid)
}

func TestEncodeSortValueOrder(t *testing.T) {
	// Encodings compare as the values do, within a kind and between kinds.
	ordered := [][]evalValue{
		nil,
		{boolValue(false)},
		{boolValue(true)},
		{intValue(math.MinInt64)},
		{intValue(-1)},
		{intValue(0)},
		{intValue(math.MaxInt64)},
		{doubleValue(math.Inf(-1))},
		{doubleValue(-2.5)},
		{doubleValue(math.Copysign(0, -1))},
		{doubleValue(0.5)},
		{doubleValue(math.Inf(1))},
		{stringValue("")},
		{stringValue("a")},
		{stringValue("ab")},
		{stringValue("b")},
		{{kind: kindOpaque}},
	}
	for i := 1; i < len(ordered); i++ {
		assert.Negative(t, bytes.Compare(encodeSortValue(ordered[i-1]), encodeSortValue(ordered[i])), "%v before %v", ordered[i-1], ordered[i])
	}
	// Only the first resolved value orders a span that carries several.
	many := []evalValue{stringValue("first"), stringValue("second")}
	assert.Equal(t, encodeSortValue(many[:1]), encodeSortValue(many))
}

func TestMakeSortingKeyResolvesAttributes(t *testing.T) {
	// The key resolves whatever the filter evaluator can, so an attribute at any level orders spans
	// once the contract admits it.
	order := []tracestore.SpanSortOrder{
		{Expression: &expression.AttributeRef{Level: expression.LevelSpan, Key: "priority"}},
		{Expression: &expression.AttributeRef{Level: expression.LevelResource, Key: "region"}},
		{Expression: &expression.FieldRef{Level: expression.LevelResource, Name: expression.ResourceFieldService}},
	}
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("region", "eu")
	resource.Attributes().PutStr("service.name", "svc")
	span := ptrace.NewSpan()
	span.Attributes().PutInt("priority", 7)
	key := makeSortingKey(filterCtx{resource: resource, span: span}, order)
	require.Len(t, key, 3)
	assert.Equal(t, encodeSortValue([]evalValue{intValue(7)}), key[0])
	assert.Equal(t, append([]byte{byte(kindString)}, "eu"...), key[1])
	assert.Equal(t, append([]byte{byte(kindString)}, "svc"...), key[2])
	assert.Equal(t, []byte{byte(kindNone)}, makeSortingKey(filterCtx{resource: pcommon.NewResource(), span: ptrace.NewSpan()}, order)[0])
}
