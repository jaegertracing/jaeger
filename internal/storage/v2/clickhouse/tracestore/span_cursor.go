// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// spanCursor is where a page ended: the sort values of its last span, one per sort term, in
// the form the continuation predicate binds. A continuation resumes strictly after them, with
// the span ID, the last term of every effective order, as the tie-breaker. The span ID is not
// unique in the spans table: a span stored twice ties on every term, and when such a pair
// straddles a page boundary the second copy is not returned. Elasticsearch makes the same
// departure from RFC 0016 §6.4 by default.
//
// TODO: write a span hash at ingestion and append it as the last sort term, so that the
// tie-breaker is unique and no copy is lost.
type spanCursor []any

// newSpanCursor is the cursor after a page that ends in row.
func newSpanCursor(row *dbmodel.SpanRow, sort []sortTerm) spanCursor {
	cursor := make(spanCursor, len(sort))
	for i, term := range sort {
		cursor[i] = term.value(row)
	}
	return cursor
}

func (c spanCursor) encode() []byte {
	// The cursor holds only integers and strings, which encode without error.
	key, _ := tracestore.EncodeSortKey(c)
	return key
}

// decodeSpanCursor reads the cursor a token carried, typed per sort term the way the
// continuation predicate binds it.
func decodeSpanCursor(raw []byte, sort []sortTerm) (spanCursor, error) {
	types := make([]tracestore.SortKeyValueType, len(sort))
	for i, term := range sort {
		types[i] = term.keyValueType()
	}
	return tracestore.DecodeSortKey(raw, types)
}
