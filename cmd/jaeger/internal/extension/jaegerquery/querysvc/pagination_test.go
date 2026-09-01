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

// TestPrepareSearchQuery_PaginationDisabled pins what a deployment that has not opted in does
// with a query carrying Pagination: it is refused before the reader is asked anything, the same
// posture TestPrepareSearchQuery_FilterDisabled pins for the filter gate.
func TestPrepareSearchQuery_PaginationDisabled(t *testing.T) {
	setPagination(t, false)

	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		ServiceName: "svc",
		Pagination:  tracestore.Pagination{PageSize: 10},
	})

	_, err := collectTraces(qs.FindTraces(context.Background(), query))
	require.ErrorIs(t, err, ErrPaginationDisabled)
	require.ErrorContains(t, err, "jaeger.query.pagination")
	assert.True(t, IsBadRequest(err), "the API layers answer 400")
	assert.False(t, next.findCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PageSizeOverridesSearchDepth pins that a PageSize takes over
// SearchDepth's role as the page bound (RFC 0014 §4), and that a Pagination with no PageToken
// never triggers the capability round trip — SearchWithoutServiceName is skipped outright here
// because ServiceName is already set, so a call to SearchCapabilities would fail this test.
func TestPrepareSearchQuery_PageSizeOverridesSearchDepth(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		ServiceName: "svc",
		SearchDepth: 20,
		Pagination:  tracestore.Pagination{PageSize: 5},
	})

	_, err := collectTraces(qs.FindTraces(context.Background(), query))
	require.NoError(t, err)
	assert.Equal(t, 5, next.gotQuery.SearchDepth, "PageSize overrides SearchDepth")
	assert.Equal(t, tracestore.Pagination{PageSize: 5}, next.gotQuery.Pagination,
		"Pagination reaches the reader untouched")
}

// TestPrepareSearchQuery_PageTokenRejectedWhenUnsupported pins RFC 0014 §6.2: a reader whose
// SearchCapabilities.Paginated is false cannot have minted a token, so a query carrying one is
// refused rather than treated as a new search.
func TestPrepareSearchQuery_PageTokenRejectedWhenUnsupported(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: false}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		Pagination: tracestore.Pagination{PageToken: "opaque-cursor"},
	})

	_, err := collectTraces(qs.FindTraces(context.Background(), query))
	require.ErrorIs(t, err, tracestore.ErrPaginationUnsupported)
	assert.True(t, IsBadRequest(err), "the API layers answer 400")
	assert.False(t, next.findCalled, "storage must not be queried")
}

// TestPrepareSearchQuery_PageTokenAcceptedWhenSupported covers the other side of the same
// check: a reader that declares Paginated keeps the token, and the ServiceName check that
// follows reuses the same capability fetch rather than asking the reader again.
func TestPrepareSearchQuery_PageTokenAcceptedWhenSupported(t *testing.T) {
	enablePagination(t)
	next := &fakeReader{batch: tracesWith("k", "v")}
	next.capabilities = &tracestore.SearchCapabilities{WithoutServiceName: true, Paginated: true}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{
		Pagination: tracestore.Pagination{PageToken: "opaque-cursor"},
	})

	_, err := collectTraces(qs.FindTraces(context.Background(), query))
	require.NoError(t, err)
	assert.True(t, next.findCalled)
	assert.Equal(t, "opaque-cursor", next.gotQuery.Pagination.PageToken)
}

// TestPrepareSearchQuery_PaginationZeroValueSkipsGate pins that a query with no Pagination at
// all takes the same path it always did: the gate is never consulted, so a deployment that has
// not enabled it sees exactly today's behavior for an unpaginated search (RFC 0014 G5).
func TestPrepareSearchQuery_PaginationZeroValueSkipsGate(t *testing.T) {
	setPagination(t, false)
	next := &fakeReader{batch: tracesWith("k", "v")}
	qs := interceptedService(next, fakeInterceptor{})
	query := searchQuery(tracestore.TraceQueryParams{ServiceName: "svc"})

	_, err := jiter.FlattenWithErrors(qs.FindTraces(context.Background(), query))
	require.NoError(t, err)
	assert.True(t, next.findCalled)
}
