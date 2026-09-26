// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/jiter"
	pagetoken "github.com/jaegertracing/jaeger/internal/proto/pagetoken/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// spanPageToken is the token a client holds after a first page of the span search q: the
// reader's cursor wrapped for the query the service dispatches, whose pagination plays no part.
func spanPageToken(t *testing.T, q tracestore.SpanQueryParams, cursor string) string {
	minted, err := pagetoken.FromSpanQuery(q)
	require.NoError(t, err)
	return encodeToken(t, minted, cursor)
}

func encodeToken(t *testing.T, minted *pagetoken.PageToken, cursor string) string {
	minted.Cursor = []byte(cursor)
	token, err := pagetoken.EncodeToString(minted)
	require.NoError(t, err)
	return token
}

// tracePageToken is spanPageToken for a trace search. The filter is finalized first, since that
// is the form the service fingerprints.
func tracePageToken(t *testing.T, q tracestore.TraceQueryParams, cursor string) string {
	if q.Filter != nil {
		finalized, err := tracestore.FinalizeFilter(q.Filter)
		require.NoError(t, err)
		q.Filter = finalized
	}
	minted, err := pagetoken.FromTraceQuery(q)
	require.NoError(t, err)
	return encodeToken(t, minted, cursor)
}

// openCursor returns the reader's cursor a token the service handed out wraps.
func openCursor(t *testing.T, token string) string {
	decoded, err := pagetoken.DecodeString(token)
	require.NoError(t, err)
	return string(decoded.Cursor)
}

func pagedSpanQuery(p tracestore.Pagination) tracestore.SpanQueryParams {
	return tracestore.SpanQueryParams{StartTimeMin: testWindowStart, StartTimeMax: testWindowEnd, Pagination: p}
}

// TestFindSpans_PageTokenRoundTrip walks a span search through two pages the way a client does:
// the reader's cursor comes back sealed in a token bound to the query, and the
// same token sent back reaches the reader as the cursor it minted (RFC 0014 §3, §5).
func TestFindSpans_PageTokenRoundTrip(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{batch: tracesWith("k", "v"), nextPageToken: "reader-cursor"}
	next.capabilities = &tracestore.SearchCapabilities{SpanSearch: true, Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{})

	first, err := collectSpans(qs.FindSpans(context.Background(), searchSpansQuery(pagedSpanQuery(tracestore.Pagination{PageSize: 10}))))
	require.NoError(t, err)
	require.Len(t, first, 1)
	token, err := pagetoken.DecodeString(first[0].NextPageToken)
	require.NoError(t, err)
	assert.Equal(t, "reader-cursor", string(token.Cursor))
	assert.NotEmpty(t, token.Fingerprint)

	_, err = collectSpans(qs.FindSpans(context.Background(), searchSpansQuery(pagedSpanQuery(tracestore.Pagination{PageSize: 10, PageToken: first[0].NextPageToken}))))
	require.NoError(t, err)
	assert.Equal(t, tracestore.Pagination{PageSize: 10, PageToken: "reader-cursor"}, next.gotSpanQuery.Pagination,
		"the reader gets its own cursor back, not the client's token")
}

// TestFindSpans_PageTokenRefused pins the tokens the service does not exchange for a cursor: one
// that does not decode and one minted for a different query. Each is the caller's mistake, so it
// is a bad request, and the reader is never asked (RFC 0014 §3.2).
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
		{name: "another query", token: spanPageToken(t, other, "c"), want: "different query"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &fakeReader{}
			next.capabilities = &tracestore.SearchCapabilities{SpanSearch: true, Paginated: true}
			qs := NewQueryService(next, nil, QueryServiceOptions{})
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

// TestFindSpans_PageTokenBoundToTheDispatchedQuery is the span-search counterpart of
// TestFindTraceSummaries_PageTokenBoundToTheDispatchedQuery: the fingerprint is taken after an
// interceptor's rewrite, so page two of the same request continues and a token minted for the
// caller's own query is refused.
func TestFindSpans_PageTokenBoundToTheDispatchedQuery(t *testing.T) {
	enablePagination(t)
	enableStructuredFilters(t)
	next := &fakeReader{batch: tracesWith("k", "v"), nextPageToken: "reader-cursor"}
	next.capabilities = filterCapableBackend()
	next.capabilities.Paginated = true
	qs := interceptedService(next, fakeInterceptor{
		onSpanQuery: func(q queryinterceptor.SpanQuery) (queryinterceptor.SpanQuery, error) {
			q.Filter = serviceFilter("gated")
			return q, nil
		},
	})
	request := func(token string) SpanQueryParams {
		q := pagedSpanQuery(tracestore.Pagination{PageSize: 10, PageToken: token})
		q.Filter = serviceFilter("original")
		return searchSpansQuery(q)
	}

	first, err := collectSpans(qs.FindSpans(context.Background(), request("")))
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, serviceFilter("gated"), next.gotSpanQuery.Filter, "the interceptor's rewrite reached the reader")

	_, err = collectSpans(qs.FindSpans(context.Background(), request(first[0].NextPageToken)))
	require.NoError(t, err)
	assert.Equal(t, "reader-cursor", next.gotSpanQuery.Pagination.PageToken)

	original := request("").SpanQueryParams
	_, err = collectSpans(qs.FindSpans(context.Background(), request(spanPageToken(t, original, "reader-cursor"))))
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
}

// TestFindSpans_NoTokenWhereNoneCanBeResumed pins that a span search mints a token only where
// the query service would accept it back: with the gate off, or against a reader that cannot
// paginate, the reader's cursor is dropped and the page carries no token, since a token the
// server would refuse on the next request is worse than none (RFC 0014 §6.2).
func TestFindSpans_NoTokenWhereNoneCanBeResumed(t *testing.T) {
	cases := []struct {
		name string
		gate bool
		caps tracestore.SearchCapabilities
	}{
		{name: "gate off", gate: false, caps: tracestore.SearchCapabilities{SpanSearch: true, Paginated: true}},
		{name: "reader cannot paginate", gate: true, caps: tracestore.SearchCapabilities{SpanSearch: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setPagination(t, tc.gate)
			next := &fakeReader{batch: tracesWith("k", "v"), nextPageToken: "reader-cursor"}
			next.capabilities = &tc.caps
			qs := NewQueryService(next, nil, QueryServiceOptions{})

			out, err := collectSpans(qs.FindSpans(context.Background(), searchSpansQuery(pagedSpanQuery(tracestore.Pagination{PageSize: 10}))))
			require.NoError(t, err)
			require.Len(t, out, 1)
			assert.Empty(t, out[0].NextPageToken)
		})
	}
}

// TestFindTraceSummaries_PageTokenRoundTrip is TestFindSpans_PageTokenRoundTrip for the summary
// search, which is the paginated surface the UI's results list consumes (RFC 0014 §4).
func TestFindTraceSummaries_PageTokenRoundTrip(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}, nextPageToken: "reader-cursor"}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{})

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

// TestFindTraceSummaries_PageTokenBoundToQuery pins that a summary token continues only the
// query it was returned for: the same cursor sent with a different service is refused.
func TestFindTraceSummaries_PageTokenBoundToQuery(t *testing.T) {
	enablePagination(t)
	minted := searchQuery(tracestore.TraceQueryParams{ServiceName: "cart"})
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{})

	_, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), searchQuery(tracestore.TraceQueryParams{
		ServiceName: "checkout",
		Pagination:  &tracestore.Pagination{PageSize: 10, PageToken: tracePageToken(t, minted.TraceQueryParams, "reader-cursor")},
	})))
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	assert.True(t, IsBadRequest(err))
	assert.False(t, next.summaryCalled)
}

// TestFindTraceSummaries_LastPageCarriesNoToken pins that a reader's empty cursor on the last
// page reaches the client as an empty token, not as a sealed envelope around nothing.
func TestFindTraceSummaries_LastPageCarriesNoToken(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	next.capabilities = &tracestore.SearchCapabilities{Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{})

	chunks, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), searchQuery(tracestore.TraceQueryParams{
		ServiceName: "cart",
		Pagination:  &tracestore.Pagination{PageSize: 10},
	})))
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Empty(t, chunks[0].NextPageToken)
}

// TestFindTraceSummaries_PageTokenBoundToTheDispatchedQuery pins that the fingerprint is taken
// over the query as the reader is dispatched it, after an interceptor's rewrite: a token minted
// on page one continues page two of the same request, and a token minted for the caller's own
// query, which the interceptor never lets through, is refused (RFC 0014 §3.2).
func TestFindTraceSummaries_PageTokenBoundToTheDispatchedQuery(t *testing.T) {
	enablePagination(t)
	enableStructuredFilters(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}, nextPageToken: "reader-cursor"}
	next.capabilities = filterCapableBackend()
	next.capabilities.Paginated = true
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
			q.Filter = serviceFilter("gated")
			return q, nil
		},
	})
	request := func(token string) TraceQueryParams {
		return searchQuery(tracestore.TraceQueryParams{
			Filter:     serviceFilter("original"),
			Pagination: &tracestore.Pagination{PageSize: 10, PageToken: token},
		})
	}

	first, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), request("")))
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, serviceFilter("gated"), next.gotSummaryQuery.Filter, "the interceptor's rewrite reached the reader")

	_, err = jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), request(first[0].NextPageToken)))
	require.NoError(t, err)
	assert.Equal(t, "reader-cursor", next.gotSummaryQuery.Pagination.PageToken)

	original := searchQuery(tracestore.TraceQueryParams{Filter: serviceFilter("original")})
	_, err = jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(),
		request(tracePageToken(t, original.TraceQueryParams, "reader-cursor"))))
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
}

// TestFindTraceSummaries_PageTokenSurvivesAttributeOrder pins that a request listing the same
// tags in another order is the same query: the API layers fill Attributes from a Go map, so a
// client re-sending one request can present it in any order, and an interceptor that adds a
// predicate turns the tags into a filter whose predicate order would otherwise follow the map.
func TestFindTraceSummaries_PageTokenSurvivesAttributeOrder(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}, nextPageToken: "reader-cursor"}
	next.capabilities = filterCapableBackend()
	next.capabilities.Paginated = true
	next.capabilities.Filter.Operators = append(next.capabilities.Filter.Operators, expression.OpAnd)
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.TraceQuery) (queryinterceptor.TraceQuery, error) {
			q.Filter = &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{q.Filter, serviceFilter("gated")}}
			return q, nil
		},
	})
	request := func(token string, kv ...string) TraceQueryParams {
		attrs := pcommon.NewMap()
		for i := 0; i < len(kv); i += 2 {
			attrs.PutStr(kv[i], kv[i+1])
		}
		return searchQuery(tracestore.TraceQueryParams{
			Attributes: attrs,
			Pagination: &tracestore.Pagination{PageSize: 10, PageToken: token},
		})
	}

	first, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), request("", "a", "1", "b", "2", "c", "3")))
	require.NoError(t, err)
	require.Len(t, first, 1)

	_, err = jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), request(first[0].NextPageToken, "c", "3", "b", "2", "a", "1")))
	require.NoError(t, err)
	assert.Equal(t, "reader-cursor", next.gotSummaryQuery.Pagination.PageToken)
}

// TestFindTraceSummaries_PageTokenRoundTripOnLegacyReader pins the round trip for a reader that
// evaluates no filter: the caller's filter is rewritten into the legacy fields before dispatch,
// and the fingerprint is taken over that rewrite on both pages, so the token still matches.
func TestFindTraceSummaries_PageTokenRoundTripOnLegacyReader(t *testing.T) {
	enablePagination(t)
	enableStructuredFilters(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}, nextPageToken: "reader-cursor"}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := NewQueryService(next, nil, QueryServiceOptions{})
	request := func(token string) TraceQueryParams {
		return searchQuery(tracestore.TraceQueryParams{
			Filter:     serviceFilter("cart"),
			Pagination: &tracestore.Pagination{PageSize: 10, PageToken: token},
		})
	}

	first, err := jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), request("")))
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, "cart", next.gotSummaryQuery.ServiceName, "the filter reached the reader as the legacy field")
	assert.Nil(t, next.gotSummaryQuery.Filter)

	_, err = jiter.CollectWithErrors(qs.FindTraceSummaries(context.Background(), request(first[0].NextPageToken)))
	require.NoError(t, err)
	assert.Equal(t, "reader-cursor", next.gotSummaryQuery.Pagination.PageToken)
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
