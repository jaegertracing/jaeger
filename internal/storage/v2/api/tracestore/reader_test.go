// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"math"
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

func TestDecodePagination(t *testing.T) {
	tests := []struct {
		name       string
		pageSize   uint32
		pageToken  string
		want       Pagination
		wantErrMsg string
	}{
		{
			name:      "page size and token",
			pageSize:  10,
			pageToken: "opaque-cursor",
			want:      Pagination{PageSize: 10, PageToken: "opaque-cursor"},
		},
		{
			name:     "page size without token",
			pageSize: 10,
			want:     Pagination{PageSize: 10},
		},
		{
			name:       "page size missing",
			pageToken:  "opaque-cursor",
			wantErrMsg: "page_size is required",
		},
		{
			name:       "both missing",
			wantErrMsg: "page_size is required",
		},
		{
			name:      "page size above max is clamped",
			pageSize:  MaxPageSize + 1,
			pageToken: "opaque-cursor",
			want:      Pagination{PageSize: MaxPageSize, PageToken: "opaque-cursor"},
		},
		{
			name:      "page size at max uint32 is clamped",
			pageSize:  math.MaxUint32,
			pageToken: "opaque-cursor",
			want:      Pagination{PageSize: MaxPageSize, PageToken: "opaque-cursor"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodePagination(tt.pageSize, tt.pageToken)
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
