// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type paginationSearch func(context.Context, tracestore.TraceQueryParams) ([]pcommon.TraceID, tracestore.PageToken, error)

func (s *StorageIntegration) testPagination(t *testing.T) {
	s.skipIfNeeded(t)
	for name, search := range map[string]paginationSearch{
		"Spans":          s.findPaginatedSpanTraceIDs,
		"TraceIDs":       s.findPaginatedTraceIDs,
		"TraceSummaries": s.findPaginatedSummaryIDs,
	} {
		t.Run(name, func(t *testing.T) {
			s.skipIfNeeded(t)
			s.assertPagination(t, search)
		})
	}
}

func (s *StorageIntegration) findPaginatedSpanTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) ([]pcommon.TraceID, tracestore.PageToken, error) {
	p := builder.Predicate{}
	filter := p.Resource().Service.Eq(query.ServiceName)
	if query.OperationName != "" {
		filter = p.And(filter, p.Span().Name.Eq(query.OperationName))
	}
	var ids []pcommon.TraceID
	var token tracestore.PageToken
	for chunk, err := range s.TraceReader.FindSpans(ctx, tracestore.SpanQueryParams{
		StartTimeMin: query.StartTimeMin,
		StartTimeMax: query.StartTimeMax,
		Filter:       filter,
		Pagination:   *query.Pagination,
	}) {
		if err != nil {
			return ids, token, err
		}
		for _, rs := range chunk.Results.ResourceSpans().All() {
			for _, ss := range rs.ScopeSpans().All() {
				for _, span := range ss.Spans().All() {
					ids = append(ids, span.TraceID())
				}
			}
		}
		token = chunk.NextPageToken
	}
	return ids, token, nil
}

func (s *StorageIntegration) findPaginatedTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) ([]pcommon.TraceID, tracestore.PageToken, error) {
	var ids []pcommon.TraceID
	var token tracestore.PageToken
	for chunk, err := range s.TraceReader.FindTraceIDs(ctx, query) {
		if err != nil {
			return ids, token, err
		}
		for i := range chunk.Results {
			ids = append(ids, chunk.Results[i].TraceID)
		}
		token = chunk.NextPageToken
	}
	return ids, token, nil
}

func (s *StorageIntegration) findPaginatedSummaryIDs(ctx context.Context, query tracestore.TraceQueryParams) ([]pcommon.TraceID, tracestore.PageToken, error) {
	var ids []pcommon.TraceID
	var token tracestore.PageToken
	for chunk, err := range s.TraceReader.FindTraceSummaries(ctx, query) {
		if err != nil {
			return ids, token, err
		}
		for i := range chunk.Results {
			ids = append(ids, chunk.Results[i].TraceID)
		}
		token = chunk.NextPageToken
	}
	return ids, token, nil
}

func (s *StorageIntegration) assertPagination(t *testing.T, search paginationSearch) {
	traces := s.Corpus.Pagination
	var want []pcommon.TraceID
	// The tied traces straddle the first page boundary, with the lower trace ID first.
	names := [...]string{
		"newest",
		"tied-lower-id",
		"tied-higher-id",
		"second-oldest",
		"oldest",
	}
	for _, name := range names {
		want = append(want, jptrace.GetTraceID(traces[name]))
	}
	first := traces["oldest"].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	query := tracestore.TraceQueryParams{
		ServiceName: "pagination-service", Attributes: pcommon.NewMap(),
		StartTimeMin: first.StartTimestamp().AsTime().Add(-time.Minute),
		StartTimeMax: first.StartTimestamp().AsTime().Add(time.Minute),
		Pagination:   &tracestore.Pagination{PageSize: uint32(len(names))},
	}
	ctx := context.Background()
	require.True(t, s.waitForCondition(t, func(t *testing.T) bool {
		ids, _, err := search(ctx, query)
		if err != nil {
			t.Log(err)
			return false
		}
		return len(ids) == len(want)
	}), "the pagination corpus must be searchable before paging begins")

	const pageSize = 2
	query.Pagination.PageSize = pageSize
	var firstToken tracestore.PageToken
	for offset := 0; offset < len(want); offset += pageSize {
		ids, token, err := search(ctx, query)
		require.NoError(t, err)
		require.Equal(t, want[offset:min(offset+pageSize, len(want))], ids)
		if offset == 0 {
			firstToken = token
		}
		if offset+len(ids) < len(want) {
			require.NotEmpty(t, token)
		} else {
			require.Empty(t, token)
		}
		query.Pagination.PageToken = token
	}
	for _, tc := range []struct {
		name      string
		token     tracestore.PageToken
		operation string
		message   string
	}{
		{name: "MalformedToken", token: "not-a-token!", message: "not valid base64"},
		{name: "DifferentQuery", token: firstToken, operation: "another-operation", message: "different query"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := query
			q.OperationName = tc.operation
			q.Pagination = &tracestore.Pagination{PageSize: 2, PageToken: tc.token}
			ids, token, err := search(ctx, q)
			require.ErrorContains(t, err, tracestore.ErrPaginationInvalid.Error())
			require.ErrorContains(t, err, tc.message)
			assert.Empty(t, ids)
			assert.Empty(t, token)
		})
	}
	t.Run("EmptyPage", func(t *testing.T) {
		query.OperationName = "another-operation"
		query.Pagination.PageToken = ""
		ids, token, err := search(ctx, query)
		require.NoError(t, err)
		assert.Empty(t, ids)
		assert.Empty(t, token)
	})
}
