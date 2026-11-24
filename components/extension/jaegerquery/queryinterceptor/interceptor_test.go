// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
)

// traceOnlyInterceptor is the shape of an implementation that gates trace searches only and
// embeds the mixin to refuse span searches.
type traceOnlyInterceptor struct {
	queryinterceptor.UnsupportedSpanSearch
}

// TestUnsupportedSpanSearch_RefusesBothHooks pins that the mixin fails closed: a span search
// is refused before it runs, and a page that somehow reached the result hook is refused too.
func TestUnsupportedSpanSearch_RefusesBothHooks(t *testing.T) {
	var i traceOnlyInterceptor
	query := queryinterceptor.SpanQuery{}

	ctx, gotQuery, err := i.OnSpanQuery(t.Context(), query)
	require.ErrorIs(t, err, queryinterceptor.ErrSpanSearchUnsupported)
	assert.Equal(t, t.Context(), ctx, "the inbound context is returned unchanged")
	assert.Equal(t, query, gotQuery)

	page := ptrace.NewTraces()
	ctx, gotPage, err := i.OnSpanResult(t.Context(), page)
	require.ErrorIs(t, err, queryinterceptor.ErrSpanSearchUnsupported)
	assert.Equal(t, t.Context(), ctx)
	assert.Equal(t, page, gotPage)
}
