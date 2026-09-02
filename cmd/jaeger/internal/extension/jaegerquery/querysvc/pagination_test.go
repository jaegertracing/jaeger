// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"

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
