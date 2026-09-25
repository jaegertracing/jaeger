// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc/pagetoken"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// spanPageToken is the token a client holds after a first page of the span search q: the
// reader's cursor sealed for the query the service dispatches, whose pagination plays no part.
func spanPageToken(t *testing.T, storage string, q tracestore.SpanQueryParams, cursor string) string {
	fingerprint, err := pagetoken.SpanQuery(q)
	require.NoError(t, err)
	return pagetoken.Seal(pagetoken.Token{Storage: storage, Fingerprint: fingerprint, Cursor: cursor})
}

// tracePageToken is spanPageToken for a trace search. The filter is finalized first, since that
// is the form the service fingerprints.
func tracePageToken(t *testing.T, storage string, q tracestore.TraceQueryParams, cursor string) string {
	if q.Filter != nil {
		finalized, err := tracestore.FinalizeFilter(q.Filter)
		require.NoError(t, err)
		q.Filter = finalized
	}
	fingerprint, err := pagetoken.TraceQuery(q)
	require.NoError(t, err)
	return pagetoken.Seal(pagetoken.Token{Storage: storage, Fingerprint: fingerprint, Cursor: cursor})
}

// openCursor returns the reader's cursor a token the service handed out wraps.
func openCursor(t *testing.T, token string) string {
	opened, err := pagetoken.Open(token)
	require.NoError(t, err)
	return opened.Cursor
}

func pagedSpanQuery(p tracestore.Pagination) tracestore.SpanQueryParams {
	return tracestore.SpanQueryParams{StartTimeMin: testWindowStart, StartTimeMax: testWindowEnd, Pagination: p}
}

// TestFindSpans_PageTokenRoundTrip walks a span search through two pages the way a client does:
// the reader's cursor comes back sealed in a token bound to the storage and the query, and the
// same token sent back reaches the reader as the cursor it minted (RFC 0014 §3, §5).
func TestFindSpans_PageTokenRoundTrip(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{batch: tracesWith("k", "v"), nextPageToken: "reader-cursor"}
	next.capabilities = &tracestore.SearchCapabilities{SpanSearch: true, Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{TraceStorageName: "primary"})

	first, err := collectSpans(qs.FindSpans(context.Background(), searchSpansQuery(pagedSpanQuery(tracestore.Pagination{PageSize: 10}))))
	require.NoError(t, err)
	require.Len(t, first, 1)
	token, err := pagetoken.Open(first[0].NextPageToken)
	require.NoError(t, err)
	assert.Equal(t, "primary", token.Storage)
	assert.Equal(t, "reader-cursor", token.Cursor)
	assert.NotEmpty(t, token.Fingerprint)

	_, err = collectSpans(qs.FindSpans(context.Background(), searchSpansQuery(pagedSpanQuery(tracestore.Pagination{PageSize: 10, PageToken: first[0].NextPageToken}))))
	require.NoError(t, err)
	assert.Equal(t, tracestore.Pagination{PageSize: 10, PageToken: "reader-cursor"}, next.gotSpanQuery.Pagination,
		"the reader gets its own cursor back, not the client's token")
}

// TestFindSpans_PageTokenRefused pins the tokens the service does not exchange for a cursor: one
// that does not decode, one another storage minted, and one minted for a different query. Each
// is the caller's mistake, so it is a bad request, and the reader is never asked (RFC 0014 §3.2).
func TestFindSpans_PageTokenRefused(t *testing.T) {
	enablePagination(t)
	query := pagedSpanQuery(tracestore.Pagination{PageSize: 10})
	other := query
	other.StartTimeMax = other.StartTimeMax.Add(1)
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{name: "malformed", token: "not-a-token!", want: "not valid base64"},
		{name: "another storage", token: spanPageToken(t, "archive", query, "c"), want: "different storage"},
		{name: "another query", token: spanPageToken(t, "primary", other, "c"), want: "different query"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &fakeReader{}
			next.capabilities = &tracestore.SearchCapabilities{SpanSearch: true, Paginated: true}
			qs := NewQueryService(next, nil, QueryServiceOptions{TraceStorageName: "primary"})
			paged := query
			paged.Pagination.PageToken = tc.token

			_, err := collectSpans(qs.FindSpans(context.Background(), searchSpansQuery(paged)))
			require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
			require.ErrorContains(t, err, tc.want)
			assert.True(t, IsBadRequest(err))
			assert.False(t, next.findCalled, "storage must not be queried")
		})
	}
}

// TestFindTraceSummaries_PageTokenRoundTrip is TestFindSpans_PageTokenRoundTrip for the summary
// search, which is the paginated surface the UI's results list consumes (RFC 0014 §4).
func TestFindTraceSummaries_PageTokenRoundTrip(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}, nextPageToken: "reader-cursor"}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{TraceStorageName: "primary"})

	first, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(),
		searchQuery(tracestore.TraceQueryParams{Pagination: &tracestore.Pagination{PageSize: 10}})))
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, "reader-cursor", openCursor(t, first[0].NextPageToken))

	sent := &tracestore.Pagination{PageSize: 10, PageToken: first[0].NextPageToken}
	_, err = jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), searchQuery(tracestore.TraceQueryParams{Pagination: sent})))
	require.NoError(t, err)
	assert.Equal(t, &tracestore.Pagination{PageSize: 10, PageToken: "reader-cursor"}, next.gotSummaryQuery.Pagination)
	assert.Equal(t, first[0].NextPageToken, sent.PageToken, "the caller's request is left as sent")
}

// TestFindTraceSummaries_PageTokenBoundToQuery pins that a summary token continues only the query
// it was returned for: the same cursor sent with a different service is refused.
func TestFindTraceSummaries_PageTokenBoundToQuery(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{})
	minted := searchQuery(tracestore.TraceQueryParams{ServiceName: "cart"})
	token := tracePageToken(t, "", minted.TraceQueryParams, "reader-cursor")

	_, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), searchQuery(tracestore.TraceQueryParams{
		ServiceName: "checkout",
		Pagination:  &tracestore.Pagination{PageSize: 10, PageToken: token},
	})))
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	assert.False(t, next.summaryCalled)
}

// TestFindTraceSummaries_UnpaginatedSearchGetsNoToken pins that a search which did not ask for a
// page is answered with one page and no token, whatever the reader returned: a client that does
// not know about pagination sees exactly the pre-pagination behavior (RFC 0014 §4).
func TestFindTraceSummaries_UnpaginatedSearchGetsNoToken(t *testing.T) {
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}, nextPageToken: "reader-cursor"}
	qs := NewQueryService(next, nil, QueryServiceOptions{})

	chunks, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), searchQuery(tracestore.TraceQueryParams{})))
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Empty(t, chunks[0].NextPageToken)
}
