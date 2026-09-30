// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esclientmocks "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func spanSearchQuery() dbmodel.SpanQueryParameters {
	start := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	return dbmodel.SpanQueryParameters{
		StartTimeMin: start,
		StartTimeMax: start.Add(time.Hour),
		Filter:       (builder.Predicate{}).Resource().Service.Eq("test-service"),
		PageSize:     2,
	}
}

func orderBy(terms ...string) []tracestore.SpanSortOrder {
	var order []tracestore.SpanSortOrder
	for i := 0; i < len(terms); i += 2 {
		order = append(order, tracestore.SpanSortOrder{
			Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: terms[i]},
			Direction:  tracestore.SortDirection(terms[i+1]),
		})
	}
	return order
}

// spanHit is a document of the span search fake: the span and its sort values in the default
// order with the id tie-breaker (startTime desc, traceID asc, spanID asc, _id asc).
func spanHit(id string, traceID, spanID string, startTime uint64) esclient.SearchHit {
	src, err := json.Marshal(dbmodel.Span{TraceID: dbmodel.TraceID(traceID), SpanID: dbmodel.SpanID(spanID), StartTime: startTime})
	if err != nil {
		panic(err)
	}
	return esclient.SearchHit{
		Source: src,
		Sort:   sortValues(strconv.FormatUint(startTime, 10), `"`+traceID+`"`, `"`+spanID+`"`, `"`+id+`"`),
	}
}

func sortValues(values ...string) []json.RawMessage {
	out := make([]json.RawMessage, len(values))
	for i, v := range values {
		out[i] = json.RawMessage(v)
	}
	return out
}

func spanIDs(spans []dbmodel.Span) []string {
	ids := make([]string, len(spans))
	for i := range spans {
		ids[i] = string(spans[i].SpanID)
	}
	return ids
}

func TestSpanReader_FindSpans_LastPage(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Run(func(args mock.Arguments) {
			req := args.Get(2).(esclient.SearchRequest)
			// One more than the page tells the reader whether another page exists.
			assert.Equal(t, 3, req.Size)
			assert.Equal(t, []esclient.SortOrder{
				{Field: startTimeField, Order: "desc"},
				{Field: traceIDField, Order: "asc"},
				{Field: spanIDField, Order: "asc"},
				{Field: idField, Order: "asc"},
			}, req.Sort)
			assert.Empty(t, req.SearchAfter)
		}).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
			spanHit("d1", "t1", "s1", 20), spanHit("d2", "t1", "s2", 10),
		}}}, nil)
		page, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
		require.NoError(t, err)
		assert.Equal(t, []string{"s1", "s2"}, spanIDs(page.Spans))
		assert.Empty(t, page.NextCursor, "a page that is not full is the last one")
	})
}

// TestSpanReader_FindSpans_PageCutByResultWindow covers a page size at or past the engine's
// result window, which the reader cannot read one past: the page is cut one short of the
// window and still carries a cursor.
func TestSpanReader_FindSpans_PageCutByResultWindow(t *testing.T) {
	searcher := esclientmocks.NewSearcher(t)
	reader := newSnapshotReader(searcher) // MaxDocCount 100
	hits := make([]esclient.SearchHit, 100)
	for i := range hits {
		hits[i] = spanHit(fmt.Sprintf("d%d", i), "t1", fmt.Sprintf("%016x", i), 1000)
	}
	searcher.On("Search", mock.Anything, mock.Anything, mock.MatchedBy(func(req esclient.SearchRequest) bool {
		return req.Size == 100
	})).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: hits}}, nil)
	query := spanSearchQuery()
	query.PageSize = 500
	page, err := reader.FindSpans(context.Background(), query)
	require.NoError(t, err)
	assert.Len(t, page.Spans, 99)
	assert.NotEmpty(t, page.NextCursor)
}

func TestSpanReader_FindSpans_CursorIsLastSortKey(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
			spanHit("d1", "t1", "s1", 20), spanHit("d2", "t1", "s2", 10), spanHit("d3", "t1", "s2", 10),
		}}}, nil)
		page, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
		require.NoError(t, err)
		assert.Equal(t, []string{"s1", "s2"}, spanIDs(page.Spans))
		// The cursor is the last returned hit's sort values, id included, so the next page
		// resumes at the tied copy d3 rather than skipping it.
		assert.JSONEq(t, `[10,"t1","s2","d2"]`, string(page.NextCursor))
	})
}

func TestSpanReader_FindSpans_Continuation(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Run(func(args mock.Arguments) {
			req := args.Get(2).(esclient.SearchRequest)
			assert.Equal(t, []any{json.RawMessage(`10`), json.RawMessage(`"t1"`), json.RawMessage(`"s2"`), json.RawMessage(`"d2"`)}, req.SearchAfter)
		}).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
			spanHit("d3", "t1", "s2", 10), spanHit("d5", "t2", "s1", 5),
		}}}, nil)
		query := spanSearchQuery()
		query.Cursor = []byte(`[10,"t1","s2","d2"]`)
		page, err := r.reader.FindSpans(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, []string{"s2", "s1"}, spanIDs(page.Spans))
		assert.Empty(t, page.NextCursor)
	})
}

// TestSpanReader_FindSpans_WithoutIDTieBreak covers the reader configured not to sort on _id:
// the sort and the cursor carry the ordering terms alone.
func TestSpanReader_FindSpans_WithoutIDTieBreak(t *testing.T) {
	searcher := esclientmocks.NewSearcher(t)
	reader := newSnapshotReader(searcher)
	reader.spanSearchTieBreakByID = false
	searcher.On("Search", mock.Anything, mock.Anything, mock.MatchedBy(func(req esclient.SearchRequest) bool {
		return len(req.Sort) == 3 && len(req.SearchAfter) == 3
	})).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
		{Source: exampleESSpan, Sort: sortValues(`20`, `"t1"`, `"s1"`)},
		{Source: exampleESSpan, Sort: sortValues(`10`, `"t1"`, `"s2"`)},
		{Source: exampleESSpan, Sort: sortValues(`10`, `"t1"`, `"s2"`)},
	}}}, nil)
	query := spanSearchQuery()
	query.Cursor = []byte(`[30,"t0","s0"]`)
	page, err := reader.FindSpans(context.Background(), query)
	require.NoError(t, err)
	assert.Len(t, page.Spans, 2)
	assert.JSONEq(t, `[10,"t1","s2"]`, string(page.NextCursor))
}

// TestSpanReader_FindSpans_OneDocumentLimit covers a max_doc_count of one, which cannot tell a
// full page from the end of the results and so is raised to two.
func TestSpanReader_FindSpans_OneDocumentLimit(t *testing.T) {
	searcher := esclientmocks.NewSearcher(t)
	reader := newSnapshotReader(searcher)
	reader.maxDocCount = 1
	searcher.On("Search", mock.Anything, mock.Anything, mock.MatchedBy(func(req esclient.SearchRequest) bool {
		return req.Size == 2
	})).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
		spanHit("d1", "t1", "s1", 20), spanHit("d2", "t1", "s2", 10),
	}}}, nil)
	page, err := reader.FindSpans(context.Background(), spanSearchQuery())
	require.NoError(t, err)
	assert.Equal(t, []string{"s1"}, spanIDs(page.Spans))
	assert.NotEmpty(t, page.NextCursor)
}

func TestSpanReader_FindSpans_ExplicitOrder(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).Run(func(args mock.Arguments) {
			req := args.Get(2).(esclient.SearchRequest)
			// The explicit term leads and the default tie-breakers follow it.
			assert.Equal(t, []esclient.SortOrder{
				{Field: durationField, Order: "desc"},
				{Field: startTimeField, Order: "desc"},
				{Field: traceIDField, Order: "asc"},
				{Field: spanIDField, Order: "asc"},
				{Field: idField, Order: "asc"},
			}, req.Sort)
		}).Return(&esclient.SearchResponse{}, nil)
		query := spanSearchQuery()
		query.OrderBy = orderBy("duration", "desc")
		page, err := r.reader.FindSpans(context.Background(), query)
		require.NoError(t, err)
		assert.Empty(t, page.Spans)
		assert.Empty(t, page.NextCursor)
	})
}

func TestSpanReader_FindSpans_RefusedBeforeSearching(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query func(q *dbmodel.SpanQueryParameters)
		err   error
	}{
		{"NoTimeRange", func(q *dbmodel.SpanQueryParameters) { q.StartTimeMin, q.StartTimeMax = time.Time{}, time.Time{} }, ErrStartAndEndTimeNotSet},
		{"InvertedTimeRange", func(q *dbmodel.SpanQueryParameters) { q.StartTimeMin, q.StartTimeMax = q.StartTimeMax, q.StartTimeMin }, ErrStartTimeMinGreaterThanMax},
		{"NoPageSize", func(q *dbmodel.SpanQueryParameters) { q.PageSize = 0 }, tracestore.ErrPaginationInvalid},
		{"UnknownOrderField", func(q *dbmodel.SpanQueryParameters) { q.OrderBy = orderBy("name", "asc") }, tracestore.ErrSpanOrderInvalid},
		{"MalformedCursor", func(q *dbmodel.SpanQueryParameters) { q.Cursor = []byte("not json") }, tracestore.ErrPaginationInvalid},
		{"CursorTooShort", func(q *dbmodel.SpanQueryParameters) { q.Cursor = []byte(`[1,"t","s"]`) }, tracestore.ErrPaginationInvalid},
		{"CursorWithObjectForTime", func(q *dbmodel.SpanQueryParameters) { q.Cursor = []byte(`[{},"t","s","d"]`) }, tracestore.ErrPaginationInvalid},
		{"CursorWithNullForID", func(q *dbmodel.SpanQueryParameters) { q.Cursor = []byte(`[1,"t",null,"d"]`) }, tracestore.ErrPaginationInvalid},
		{"CursorWithNumberForID", func(q *dbmodel.SpanQueryParameters) { q.Cursor = []byte(`[1,2,"s","d"]`) }, tracestore.ErrPaginationInvalid},
		{"CursorOfAnotherOrder", func(q *dbmodel.SpanQueryParameters) {
			q.OrderBy = orderBy("duration", "desc")
			q.Cursor = []byte(`[1,"t","s","d"]`)
		}, tracestore.ErrPaginationInvalid},
		{"UnservableFilter", func(q *dbmodel.SpanQueryParameters) {
			q.Filter = (builder.Predicate{}).Scope().Attr("x").Eq("y")
		}, tracestore.ErrFilterUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSpanReader(t, func(r *spanReaderTest) {
				query := spanSearchQuery()
				tc.query(&query)
				_, err := r.reader.FindSpans(context.Background(), query)
				require.ErrorIs(t, err, tc.err)
				r.searcher.AssertNotCalled(t, "Search")
			})
		})
	}
}

func TestSpanReader_FindSpans_SearchErrors(t *testing.T) {
	t.Run("SearchFails", func(t *testing.T) {
		withSpanReader(t, func(r *spanReaderTest) {
			mockSearchService(r).Return(nil, errors.New("read error"))
			_, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
			require.ErrorContains(t, err, "read error")
		})
	})
	t.Run("MalformedDocument", func(t *testing.T) {
		withSpanReader(t, func(r *spanReaderTest) {
			mockSearchService(r).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
				{Source: []byte(`{"traceID": 1 bad json`)},
			}}}, nil)
			_, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
			require.ErrorContains(t, err, "marshalling JSON to span object failed")
		})
	})
	t.Run("MissingSortValues", func(t *testing.T) {
		withSpanReader(t, func(r *spanReaderTest) {
			// A full page whose last hit carries no sort values cannot be resumed from.
			hits := []esclient.SearchHit{spanHit("d1", "t1", "s1", 20), {Source: exampleESSpan}, spanHit("d3", "t1", "s3", 5)}
			mockSearchService(r).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: hits}}, nil)
			_, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
			require.ErrorContains(t, err, "returned 0 sort values for 4 sort fields")
		})
	})
}

func TestSortFieldRefusesAnEscapedTerm(t *testing.T) {
	assert.Panics(t, func() { sortField(orderBy("name", "asc")[0]) })
}

// TestSpanSearchRequestSnapshots freezes the wire format of the span search: the first page in
// the default order, and a continuation in a mixed-direction order that resumes after the
// previous page with search_after. Fixed query times keep the range filters
// deterministic.
func TestSpanSearchRequestSnapshots(t *testing.T) {
	firstPage := spanSearchQuery()
	continuation := spanSearchQuery()
	continuation.OrderBy = orderBy("duration", "desc", "traceID", "asc")
	continuation.Cursor = []byte(`[2000000,"000000000000000000000000000000ab",1577934245000000,"00000000000000cd","doc-1"]`)

	snapshots := map[string]map[es.BackendVersion]string{"find_spans": {}, "find_spans_continuation": {}}
	for _, version := range es.AllVersions {
		rec := dataRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)
		esClient, err := esclient.NewClient(context.Background(), &config.Configuration{Servers: []string{server.URL}, Version: uint(version)}, zap.NewNop(), nil)
		require.NoError(t, err)
		reader := newSnapshotReader(esclient.SearchClient{Client: esClient})
		for name, query := range map[string]dbmodel.SpanQueryParameters{"find_spans": firstPage, "find_spans_continuation": continuation} {
			rec.Reset()
			_, err = reader.FindSpans(context.Background(), query)
			require.NoError(t, err)
			snapshots[name][version] = rec.Marshal(t)
		}
	}
	for name, byVersion := range snapshots {
		snapshottest.AssertByVersion(t, "testdata/"+name, byVersion)
	}
}
