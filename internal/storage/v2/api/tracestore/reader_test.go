// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestUnsupportedSpanSearch_FindSpans(t *testing.T) {
	iterations := 0
	for batch, err := range (UnsupportedSpanSearch{}).FindSpans(context.Background(), SpanQueryParams{}) {
		iterations++
		assert.Equal(t, PageChunk[ptrace.Traces]{}, batch)
		require.ErrorIs(t, err, errors.ErrUnsupported)
	}
	assert.Equal(t, 1, iterations, "expected exactly one yield carrying ErrUnsupported")
}

func TestTraceQueryParams_ReplacePaginationWithDepth(t *testing.T) {
	t.Run("no pagination is returned unchanged", func(t *testing.T) {
		query := TraceQueryParams{ServiceName: "svc", SearchDepth: 7}
		got, err := query.ReplacePaginationWithDepth()
		require.NoError(t, err)
		assert.Equal(t, query, got)
	})
	t.Run("page size becomes the depth", func(t *testing.T) {
		query := TraceQueryParams{ServiceName: "svc", Pagination: &Pagination{PageSize: 3}}
		got, err := query.ReplacePaginationWithDepth()
		require.NoError(t, err)
		assert.Equal(t, TraceQueryParams{ServiceName: "svc", SearchDepth: 3}, got)
	})
	t.Run("a token is refused", func(t *testing.T) {
		query := TraceQueryParams{Pagination: &Pagination{PageSize: 3, PageToken: "t"}}
		_, err := query.ReplacePaginationWithDepth()
		require.ErrorIs(t, err, ErrPaginationUnsupported)
		assert.ErrorContains(t, err, "do not page yet")
	})
}
