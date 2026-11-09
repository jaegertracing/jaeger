// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/json"
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// spanCursor is where a page ended: the sort values of its last span, one per sort term. A
// continuation resumes strictly after them, with the span ID, the last term of every effective
// order, as the tie-breaker. The span ID is not unique in the spans table: a span stored twice
// ties on every term, and when such a pair straddles a page boundary the second copy is not
// returned. Elasticsearch makes the same departure from RFC 0016 §6.4 by default.
//
// TODO: write a span hash at ingestion and append it as the last sort term, so that the
// tie-breaker is unique and no copy is lost.
type spanCursor struct {
	Key []any `json:"key"`
}

// newSpanCursor is the cursor after a page that ends in row.
func newSpanCursor(row *dbmodel.SpanRow, sort []sortTerm) spanCursor {
	key := make([]any, len(sort))
	for i, term := range sort {
		key[i] = term.value(row)
	}
	return spanCursor{Key: key}
}

func (c spanCursor) encode() []byte {
	// The cursor holds only integers and strings, which encode without error.
	b, _ := json.Marshal(c)
	return b
}

// decodeSpanCursor reads the cursor a token carried. A cursor is refused unless it holds one
// value per sort term, an integer for a numeric column and a string for an ID, since the
// reader never returns one shaped otherwise.
func decodeSpanCursor(raw []byte, sort []sortTerm) (spanCursor, error) {
	var wire struct {
		Key []json.RawMessage `json:"key"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil || len(wire.Key) != len(sort) {
		return spanCursor{}, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
	}
	cursor := spanCursor{Key: make([]any, len(sort))}
	for i, term := range sort {
		var err error
		switch term.column {
		case "trace_id", "id":
			var s string
			err = json.Unmarshal(wire.Key[i], &s)
			cursor.Key[i] = s
		default:
			var n int64
			err = json.Unmarshal(wire.Key[i], &n)
			cursor.Key[i] = n
		}
		if err != nil {
			return spanCursor{}, fmt.Errorf("%w: page token value %d does not fit the sort column %s", tracestore.ErrPaginationInvalid, i, term.column)
		}
	}
	return cursor, nil
}
