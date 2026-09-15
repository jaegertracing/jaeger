// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func enablePagination(t *testing.T) {
	setPagination(t, true)
}

func setPagination(t *testing.T, enabled bool) {
	original := PaginationGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(PaginationGate.ID(), enabled))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(PaginationGate.ID(), original))
	})
}

func TestPaginationGate_IsAlpha(t *testing.T) {
	assert.Equal(t, featuregate.StageAlpha, PaginationGate.Stage(),
		"Alpha is what keeps pagination off unless a deployment asks for it")
}

// TestFindTraces_RejectsPagination pins that FindTraces refuses any query carrying Pagination
// outright, before prepareSearchQuery — which FindTraceSummaries shares and which does admit
// Pagination — ever sees it. FindTraces streams whole traces with no field to carry a
// continuation token (RFC 0014 §4).
func TestFindTraces_RejectsPagination(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		ServiceName: "svc",
		Pagination:  tracestore.Pagination{PageSize: 10},
	})

	_, err := collectTraces(qs.FindTraces(context.Background(), query))
	require.ErrorIs(t, err, tracestore.ErrPaginationUnsupportedByFindTraces)
	assert.True(t, IsBadRequest(err), "the API layers answer 400")
	assert.False(t, next.findCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PaginationDisabled pins what a deployment that has not opted in does
// with a query carrying Pagination: it is refused before the reader is asked anything, the same
// posture TestPrepareSearchQuery_FilterDisabled pins for the filter gate. Driven through
// FindTraceSummaries, since FindTraces refuses Pagination unconditionally regardless of the
// gate (TestFindTraces_RejectsPagination).
func TestPrepareSearchQuery_PaginationDisabled(t *testing.T) {
	setPagination(t, false)

	next := &fakeReader{}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		ServiceName: "svc",
		Pagination:  tracestore.Pagination{PageSize: 10},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.ErrorIs(t, err, ErrPaginationDisabled)
		require.ErrorContains(t, err, "jaeger.query.pagination")
		assert.True(t, IsBadRequest(err), "the API layers answer 400")
	}
	assert.False(t, next.summaryCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PaginationMutuallyExclusiveWithSearchDepth pins RFC 0014 §4:
// page_size replaces search_depth rather than falling back to it, so a query that sets both has
// not said how many results it wants, and is refused rather than letting one silently win.
func TestPrepareSearchQuery_PaginationMutuallyExclusiveWithSearchDepth(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		ServiceName: "svc",
		SearchDepth: 20,
		Pagination:  tracestore.Pagination{PageSize: 5},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
		require.ErrorContains(t, err, "search_depth")
		assert.True(t, IsBadRequest(err), "the API layers answer 400")
	}
	assert.False(t, next.summaryCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PageSizeRequiredWhenPaginationPresent pins the other half of RFC 0014
// §4: a Pagination that leaves page_size at zero does not describe a page, so it is refused
// rather than silently drawing a bound from anywhere else.
func TestPrepareSearchQuery_PageSizeRequiredWhenPaginationPresent(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		ServiceName: "svc",
		Pagination:  tracestore.Pagination{PageToken: "opaque-cursor"},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
		require.ErrorContains(t, err, "page_size is required")
		assert.True(t, IsBadRequest(err), "the API layers answer 400")
	}
	assert.False(t, next.summaryCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PageSizeFoldedIntoSearchDepthWhenUnsupported pins the fix for a real
// bug Copilot found: a page-size-only request against a reader that cannot paginate used to
// return before the capability round trip at all, so it reached the reader with SearchDepth == 0
// and a Pagination field that reader does not consume — effectively unbounded. PageSize is now
// folded into SearchDepth and Pagination cleared (ApplyPaginationCapability), so every reader in
// the fleet today, which only understands SearchDepth, actually honors the requested bound.
func TestPrepareSearchQuery_PageSizeFoldedIntoSearchDepthWhenUnsupported(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: false}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		Pagination: tracestore.Pagination{PageSize: 15},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.NoError(t, err)
	}
	assert.True(t, next.summaryCalled)
	assert.Equal(t, tracestore.Pagination{}, next.gotSummaryQuery.Pagination, "cleared once folded")
	assert.Equal(t, 15, next.gotSummaryQuery.SearchDepth)
}

// TestPrepareSearchQuery_PageSizeOnlyKeepsPaginationWhenSupported covers the other side: a
// reader that declares Paginated keeps Pagination as sent, since it has its own field to read
// PageSize from.
func TestPrepareSearchQuery_PageSizeOnlyKeepsPaginationWhenSupported(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		Pagination: tracestore.Pagination{PageSize: 15},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.NoError(t, err)
	}
	assert.True(t, next.summaryCalled)
	assert.Equal(t, tracestore.Pagination{PageSize: 15}, next.gotSummaryQuery.Pagination)
	assert.Zero(t, next.gotSummaryQuery.SearchDepth)
}

// TestPrepareSearchQuery_PageTokenRejectedWhenUnsupported pins RFC 0014 §6.2: a reader whose
// SearchCapabilities.Paginated is false cannot have minted a token, so a query carrying one is
// refused rather than treated as a new search.
func TestPrepareSearchQuery_PageTokenRejectedWhenUnsupported(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: false}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		Pagination: tracestore.Pagination{PageSize: 20, PageToken: "opaque-cursor"},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.ErrorIs(t, err, tracestore.ErrPaginationUnsupported)
		assert.True(t, IsBadRequest(err), "the API layers answer 400")
	}
	assert.False(t, next.summaryCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PageTokenAcceptedWhenSupported covers the other side of the same
// check: a reader that declares Paginated keeps the token, and the ServiceName check that
// follows reuses the same capability fetch rather than asking the reader again.
func TestPrepareSearchQuery_PageTokenAcceptedWhenSupported(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		Pagination: tracestore.Pagination{PageSize: 20, PageToken: "opaque-cursor"},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.NoError(t, err)
	}
	assert.True(t, next.summaryCalled)
	assert.Equal(t, "opaque-cursor", next.gotSummaryQuery.Pagination.PageToken)
	assert.Equal(t, 20, next.gotSummaryQuery.Pagination.PageSize)
}

// TestPagination_SurvivesInterceptorFilterRewrite pins another Copilot finding: onQuery rebuilds
// TraceQueryParams via fromPublicQuery whenever an interceptor's Filter output differs from what
// went in (or the query already carried a Filter), and that reconstruction used to have no
// Pagination field at all, silently dropping both PageSize and PageToken. A query with both
// Filter and Pagination set must keep Pagination once an interceptor is configured, even a
// no-op one, since configuring any interceptor is what routes the query through this
// reconstruction path.
func TestPagination_SurvivesInterceptorFilterRewrite(t *testing.T) {
	enablePagination(t)
	enableStructuredFilters(t)
	filter := tag(expression.OpEq, "a", "1")
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	next.capabilities = &tracestore.SearchCapabilities{
		WithoutServiceName: true,
		Paginated:          true,
		Filter: &tracestore.FilterCapabilities{
			Levels:    []expression.Level{expression.LevelSpan},
			Operators: []expression.Operator{expression.OpEq},
		},
	}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		Filter:     filter,
		Pagination: tracestore.Pagination{PageSize: 10, PageToken: "opaque-cursor"},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.NoError(t, err)
	}
	assert.True(t, next.summaryCalled)
	assert.Equal(t, tracestore.Pagination{PageSize: 10, PageToken: "opaque-cursor"}, next.gotSummaryQuery.Pagination,
		"Pagination must not be dropped by the interceptor round trip")
}

// TestPagination_RevalidatedAfterInterceptor pins that an interceptor cannot smuggle a query
// past EnsurePaginationStandsAlone by changing SearchDepth: the check runs again on whatever the
// interceptor returns, the same posture finalizeInterceptorFilter already takes for Filter.
func TestPagination_RevalidatedAfterInterceptor(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{}
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
			q.SearchDepth = 20
			return q, nil
		},
	})
	query := searchQuery(tracestore.TraceQueryParams{
		Pagination: tracestore.Pagination{PageSize: 10},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
		require.ErrorContains(t, err, "search_depth")
	}
	assert.False(t, next.summaryCalled, "storage must not be queried")
}

// TestPagination_RevalidatedAfterInterceptorFilterRewrite covers the reconstruction path's own
// re-validation, the counterpart to TestPagination_RevalidatedAfterInterceptor for the fast
// path: an interceptor that also sets SearchDepth while rewriting Filter must not smuggle a
// mutually-exclusive query past EnsurePaginationStandsAlone via fromPublicQuery.
func TestPagination_RevalidatedAfterInterceptorFilterRewrite(t *testing.T) {
	enablePagination(t)
	enableStructuredFilters(t)
	next := &fakeReader{}
	qs := interceptedService(next, fakeInterceptor{
		onQuery: func(q queryinterceptor.Query) (queryinterceptor.Query, error) {
			q.Filter = tag(expression.OpEq, "b", "2")
			q.SearchDepth = 20
			return q, nil
		},
	})
	query := searchQuery(tracestore.TraceQueryParams{
		Filter:     tag(expression.OpEq, "a", "1"),
		Pagination: tracestore.Pagination{PageSize: 10},
	})

	for _, err := range qs.FindTraceSummaries(context.Background(), query) {
		require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
		require.ErrorContains(t, err, "search_depth")
	}
	assert.False(t, next.summaryCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PaginationZeroValueSkipsGate pins that a query with no Pagination at
// all takes the same path it always did: the gate is never consulted, so a deployment that has
// not enabled it sees exactly today's behavior for an unpaginated search (RFC 0014 G5). Driven
// through FindTraces, which is the RPC a zero Pagination must still work on.
func TestPrepareSearchQuery_PaginationZeroValueSkipsGate(t *testing.T) {
	setPagination(t, false)
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{ServiceName: "svc"})

	_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), query))
	require.NoError(t, err)
	assert.True(t, next.findCalled)
}
