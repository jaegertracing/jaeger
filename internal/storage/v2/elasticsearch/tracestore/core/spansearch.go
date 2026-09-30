// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// FindSpans returns one page of the spans matching the query, in the query's effective
// order, and the cursor that resumes the search after them (RFC 0016 §6). The cursor is the
// last hit's sort values as the engine returned them, and a continuation passes them back as
// search_after. The engine sorts and cuts the page, so a page reads one document past what
// it returns, and that extra hit only tells whether another page exists. The engine returns
// at most maxDocCount hits, the index's result window, so a page that would need more is cut
// one short of the window; a page size is a maximum, and the cursor still resumes exactly
// after the page.
func (*SpanReader) FindSpans(_ context.Context, _ dbmodel.SpanQueryParameters) (dbmodel.SpanPage, error) {
	return dbmodel.SpanPage{}, tracestore.Unsupported("FindSpans is not supported yet")
}
