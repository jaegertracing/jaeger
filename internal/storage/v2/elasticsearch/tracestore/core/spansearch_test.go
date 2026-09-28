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

// spanHit is a document of the span search fake: the span, its id, and its sort values in the
// default order (startTime desc, traceID asc, spanID asc).
func spanHit(id string, traceID, spanID string, startTime uint64) esclient.SearchHit {
	src, err := json.Marshal(dbmodel.Span{TraceID: dbmodel.TraceID(traceID), SpanID: dbmodel.SpanID(spanID), StartTime: startTime})
	if err != nil {
		panic(err)
	}
	return esclient.SearchHit{
		Index:  "jaeger-span-2020-01-02",
		ID:     id,
		Source: src,
		Sort:   []json.RawMessage{json.RawMessage(strconv.FormatUint(startTime, 10)), json.RawMessage(`"` + traceID + `"`), json.RawMessage(`"` + spanID + `"`)},
	}
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

func TestSpanReader_FindSpans_CursorCarriesTiedIDs(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// The page ends inside a run of hits sharing the whole sort key, as two copies of one
		// stored span do; the cursor must name the copies already returned.
		mockSearchService(r).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
			spanHit("d1", "t1", "s1", 20), spanHit("d2", "t1", "s2", 10), spanHit("d3", "t1", "s2", 10),
		}}}, nil)
		page, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
		require.NoError(t, err)
		assert.Equal(t, []string{"s1", "s2"}, spanIDs(page.Spans))
		var cursor spanCursor
		require.NoError(t, json.Unmarshal(page.NextCursor, &cursor))
		assert.Equal(t, []json.RawMessage{json.RawMessage(`10`), json.RawMessage(`"t1"`), json.RawMessage(`"s2"`)}, cursor.Sort)
		assert.Equal(t, []spanDoc{{Index: "jaeger-span-2020-01-02", ID: "d2"}}, cursor.Docs)
	})
}

func TestSpanReader_FindSpans_Continuation(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		// The cursor names a copy stored in an older index under the same id as one of this
		// page's hits, which is what a retry after a rollover leaves behind; both are occurrences.
		before := spanCursor{
			Sort: []json.RawMessage{json.RawMessage(`10`), json.RawMessage(`"t1"`), json.RawMessage(`"s2"`)},
			Docs: []spanDoc{{Index: "jaeger-span-2020-01-01", ID: "d3"}, {Index: "jaeger-span-2020-01-02", ID: "d2"}},
		}
		cursor, err := json.Marshal(before)
		require.NoError(t, err)
		mockSearchService(r).Run(func(args mock.Arguments) {
			req := args.Get(2).(esclient.SearchRequest)
			src, err := req.Query.Source()
			require.NoError(t, err)
			body, err := json.Marshal(src)
			require.NoError(t, err)
			// The continuation selects the keys at or after the cursor and leaves out the
			// documents the cursor names, each within its own index, so a tied copy is returned
			// and a returned one is not.
			assert.Contains(t, string(body), `"must_not":[{"bool":{"must":[{"term":{"_index":"jaeger-span-2020-01-01"}},{"ids":{"values":["d3"]}}]}},{"bool":{"must":[{"term":{"_index":"jaeger-span-2020-01-02"}},{"ids":{"values":["d2"]}}]}}]`)
			assert.Contains(t, string(body), `"range":{"startTime":{"lt":10}}`)
			assert.Contains(t, string(body), `"range":{"spanID":{"gt":"s2"}}`)
		}).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
			spanHit("d3", "t1", "s2", 10), spanHit("d4", "t1", "s2", 10), spanHit("d5", "t2", "s1", 5),
		}}}, nil)
		query := spanSearchQuery()
		query.Cursor = cursor
		page, err := r.reader.FindSpans(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, []string{"s2", "s2"}, spanIDs(page.Spans))
		var next spanCursor
		require.NoError(t, json.Unmarshal(page.NextCursor, &next))
		// The page did not move past the cursor's key, so the named documents accumulate.
		assert.Equal(t, before.Sort, next.Sort)
		assert.Equal(t, append([]spanDoc{{Index: "jaeger-span-2020-01-02", ID: "d3"}, {Index: "jaeger-span-2020-01-02", ID: "d4"}}, before.Docs...), next.Docs)
	})
}

func TestSpanReader_FindSpans_ContinuationPastCursorKey(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		cursor, err := json.Marshal(spanCursor{
			Sort: []json.RawMessage{json.RawMessage(`10`), json.RawMessage(`"t1"`), json.RawMessage(`"s2"`)},
			Docs: []spanDoc{{Index: "jaeger-span-2020-01-02", ID: "d2"}},
		})
		require.NoError(t, err)
		mockSearchService(r).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: []esclient.SearchHit{
			spanHit("d5", "t2", "s1", 5), spanHit("d6", "t2", "s2", 5), spanHit("d7", "t3", "s1", 1),
		}}}, nil)
		query := spanSearchQuery()
		query.Cursor = cursor
		page, err := r.reader.FindSpans(context.Background(), query)
		require.NoError(t, err)
		var next spanCursor
		require.NoError(t, json.Unmarshal(page.NextCursor, &next))
		// The page moved past the cursor's key, so the documents it named are dropped: they
		// no longer tie with the new last hit and excluding them would be needless.
		assert.Equal(t, []json.RawMessage{json.RawMessage(`5`), json.RawMessage(`"t2"`), json.RawMessage(`"s2"`)}, next.Sort)
		assert.Equal(t, []spanDoc{{Index: "jaeger-span-2020-01-02", ID: "d6"}}, next.Docs)
	})
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
	validCursor, err := json.Marshal(spanCursor{Sort: []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"t"`), json.RawMessage(`"s"`)}, Docs: []spanDoc{{Index: "i", ID: "d"}}})
	require.NoError(t, err)
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
		{"CursorWithoutDocuments", func(q *dbmodel.SpanQueryParameters) { q.Cursor = []byte(`{"sort":[1,"t","s"]}`) }, tracestore.ErrPaginationInvalid},
		{"CursorWithObjectForTime", func(q *dbmodel.SpanQueryParameters) {
			q.Cursor = []byte(`{"sort":[{},"t","s"],"docs":[{"index":"i","id":"d"}]}`)
		}, tracestore.ErrPaginationInvalid},
		{"CursorWithNumberForID", func(q *dbmodel.SpanQueryParameters) {
			q.Cursor = []byte(`{"sort":[1,2,"s"],"docs":[{"index":"i","id":"d"}]}`)
		}, tracestore.ErrPaginationInvalid},
		{"CursorOfAnotherOrder", func(q *dbmodel.SpanQueryParameters) {
			q.OrderBy = orderBy("duration", "desc")
			q.Cursor = validCursor
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
				{ID: "d1", Source: []byte(`{"traceID": 1 bad json`)},
			}}}, nil)
			_, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
			require.ErrorContains(t, err, "marshalling JSON to span object failed")
		})
	})
	t.Run("MissingSortValues", func(t *testing.T) {
		withSpanReader(t, func(r *spanReaderTest) {
			// A full page whose last hit carries no sort values cannot be resumed from.
			hits := []esclient.SearchHit{spanHit("d1", "t1", "s1", 20), {ID: "d2", Source: exampleESSpan}, spanHit("d3", "t1", "s3", 5)}
			mockSearchService(r).Return(&esclient.SearchResponse{Hits: esclient.HitsResult{Hits: hits}}, nil)
			_, err := r.reader.FindSpans(context.Background(), spanSearchQuery())
			require.ErrorContains(t, err, "returned 0 sort values for 3 sort fields")
		})
	})
}

func TestSortFieldRefusesAnEscapedTerm(t *testing.T) {
	assert.Panics(t, func() { sortField(orderBy("name", "asc")[0]) })
}

func TestSameSortValues(t *testing.T) {
	raw := func(values ...string) []json.RawMessage {
		out := make([]json.RawMessage, len(values))
		for i, v := range values {
			out[i] = json.RawMessage(v)
		}
		return out
	}
	assert.True(t, sameSortValues(raw(`10`, ` "a" `), raw(`10`, `"a"`)), "formatting does not matter")
	assert.False(t, sameSortValues(raw(`10`), raw(`10`, `"a"`)))
	assert.False(t, sameSortValues(raw(`10`, `"a"`), raw(`10`, `"b"`)))
	assert.False(t, sameSortValues(raw(`not json`), raw(`10`)))
}

// TestSpanSearchRequestSnapshots freezes the wire format of the span search: the first page in
// the default order, and a continuation in a mixed-direction order whose keyset predicate and
// id exclusion resume after the previous page. Fixed query times keep the range filters
// deterministic.
func TestSpanSearchRequestSnapshots(t *testing.T) {
	firstPage := spanSearchQuery()
	continuation := spanSearchQuery()
	continuation.OrderBy = orderBy("duration", "desc", "traceID", "asc")
	cursor, err := json.Marshal(spanCursor{
		Sort: []json.RawMessage{json.RawMessage(`2000000`), json.RawMessage(`"000000000000000000000000000000ab"`), json.RawMessage(`1577934245000000`), json.RawMessage(`"00000000000000cd"`)},
		Docs: []spanDoc{{Index: "jaeger-span-2020-01-01", ID: "doc-1"}, {Index: "jaeger-span-2020-01-02", ID: "doc-2"}},
	})
	require.NoError(t, err)
	continuation.Cursor = cursor

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
