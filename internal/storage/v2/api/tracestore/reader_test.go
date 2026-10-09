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
