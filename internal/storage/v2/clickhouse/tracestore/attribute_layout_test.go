// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
)

// TestAttributeLayout_SelectSpans_Map snapshots the statement that reads spans from a table in
// the map layout. The statement of the nested layout is in the snapshots of the reader tests.
func TestAttributeLayout_SelectSpans_Map(t *testing.T) {
	reader := NewReader(&clickhousetest.Driver{}, ReaderConfig{AttributeLayout: AttributeLayoutMap})

	query, _ := reader.buildGetTracesQuery(tracestore.GetTraceParams{})

	clickhousetest.VerifyQuerySnapshot(t, query)
}
