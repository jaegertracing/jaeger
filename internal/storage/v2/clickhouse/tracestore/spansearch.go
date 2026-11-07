// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"iter"
	"slices"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// spanKey is the default sort key of a span search, (startTime desc, traceID asc, spanID asc)
// (RFC 0016 §6.1), read from the row the way the spans table stores it.
type spanKey struct {
	// startTime is the span's start as Unix nanoseconds, so that keys compare with ==.
	startTime int64
	traceID   pcommon.TraceID
	spanID    pcommon.SpanID
}

// spanKeySize is the encoded size of a spanKey: the start time as Unix nanoseconds, then the
// two IDs.
const spanKeySize = 8 + 16 + 8

// spanCursor is where a page ended: the key of its last span and the content hashes of the rows
// with that key the pages so far have returned. The spans table has no column that names one
// row, and ClickHouse does not promise the same order among rows that tie on the whole key from
// one query to the next, so a count of tied rows would not say which of them a page already
// holds (RFC 0016 §6.4). The hashes do: the continuation re-reads the tied rows and drops one
// occurrence per listed hash, so a span stored twice comes back twice, and two rows that tie on
// the key but differ in content each come back once, whichever order ClickHouse returns them in.
type spanCursor struct {
	key      spanKey
	returned []uint64
}

// maxCursorHashes bounds the hashes a cursor may list, and with them the rows a continuation
// reads beyond its page. A page lists at most its own size of hashes, and the list grows only
// while consecutive pages end inside one run of rows tying on the whole key, so the bound is
// reached only by a run of duplicate writes longer than the largest page; a token past it is
// refused rather than let a forged token size the fetch.
const maxCursorHashes = int(tracestore.MaxPageSize)

func (c spanCursor) encode() []byte {
	buf := make([]byte, 0, spanKeySize+8*len(c.returned))
	buf, _ = binary.Append(buf, binary.BigEndian, c.key.startTime)
	buf = append(buf, c.key.traceID[:]...)
	buf = append(buf, c.key.spanID[:]...)
	for _, h := range c.returned {
		buf = binary.BigEndian.AppendUint64(buf, h)
	}
	return buf
}

func decodeSpanCursor(raw []byte) (spanCursor, error) {
	if len(raw) < spanKeySize || (len(raw)-spanKeySize)%8 != 0 {
		return spanCursor{}, fmt.Errorf("%w: page token does not carry a span position", tracestore.ErrPaginationInvalid)
	}
	if (len(raw)-spanKeySize)/8 > maxCursorHashes {
		return spanCursor{}, fmt.Errorf("%w: page token lists more than %d spans at its position", tracestore.ErrPaginationInvalid, maxCursorHashes)
	}
	var c spanCursor
	// A fixed-size integer always decodes from a buffer of at least its size.
	_, _ = binary.Decode(raw, binary.BigEndian, &c.key.startTime)
	copy(c.key.traceID[:], raw[8:24])
	copy(c.key.spanID[:], raw[24:spanKeySize])
	for rest := raw[spanKeySize:]; len(rest) > 0; rest = rest[8:] {
		c.returned = append(c.returned, binary.BigEndian.Uint64(rest))
	}
	return c, nil
}

// foundSpan is one row of a span search page: its key and the row as scanned.
type foundSpan struct {
	key spanKey
	row *dbmodel.SpanRow
}

// hash is the content hash a cursor lists for a row at its key. It hashes the row as the spans
// table returns it, whose columns are strings and arrays in stored order, so two reads of one
// row hash alike and so do two rows a duplicate write produced. The converted span would not do:
// a map-valued attribute is rebuilt through a Go map, whose order differs between reads.
func (s foundSpan) hash() uint64 {
	// The row holds only strings, numbers, times and slices of those, which encode without error.
	b, _ := json.Marshal(s.row)
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

// FindSpans returns one page of the spans matching query, in the default order, with the token
// that resumes the search after them (RFC 0016 §6). It selects the span rows directly, with the
// same predicates the trace search assembles and without the trace-ID subquery, ordered by
// spanKey and cut at the page size; the cursor inside the token is the last row's key, and a
// continuation reads from that key onward. The reader orders by the default key only, so an
// explicit order is refused (SpanSorting is false).
func (r *Reader) FindSpans(ctx context.Context, query tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		chunk, err := r.findSpanPage(ctx, query)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		yield(chunk, nil)
	}
}

func (r *Reader) findSpanPage(ctx context.Context, query tracestore.SpanQueryParams) (tracestore.PageChunk[ptrace.Traces], error) {
	var none tracestore.PageChunk[ptrace.Traces]
	// The query service refuses an explicit order before dispatch, but a reader reached directly
	// must refuse it too, since the SQL orders by the default key alone.
	capabilities, _ := r.SearchCapabilities(ctx)
	if err := capabilities.ValidateSpanSorting(query.OrderBy); err != nil {
		return none, err
	}
	fingerprint, err := query.Fingerprint()
	if err != nil {
		return none, err
	}
	var after *spanCursor
	if query.Pagination.PageToken != "" {
		raw, err := query.Pagination.PageToken.Cursor(fingerprint)
		if err != nil {
			return none, err
		}
		cursor, err := decodeSpanCursor(raw)
		if err != nil {
			return none, err
		}
		after = &cursor
	}
	// The query service clamps the page size (RFC 0014 §4); a caller reaching the reader
	// directly gets the same bound.
	pageSize := min(query.Pagination.PageSize, tracestore.MaxPageSize)
	if pageSize == 0 {
		return none, fmt.Errorf("%w: page size must be greater than 0", tracestore.ErrPaginationInvalid)
	}
	q, args, err := r.buildFindSpansQuery(ctx, query, after, pageSize)
	if err != nil {
		return none, fmt.Errorf("failed to build query: %w", err)
	}
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return none, fmt.Errorf("failed to query spans: %w", err)
	}
	// One row beyond the page tells whether another page exists.
	found, err := readFoundSpans(rows, after, pageSize+1)
	if err != nil {
		return none, err
	}
	more := uint64(len(found)) > uint64(pageSize)
	if more {
		found = found[:pageSize]
	}
	chunk := tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces()}
	if more {
		next := nextSpanCursor(found, after)
		if chunk.NextPageToken, err = tracestore.NewPageToken(fingerprint, next.encode()); err != nil {
			return none, err
		}
	}
	// Every span keeps its own ResourceSpans, so the page keeps the SQL order even where
	// consecutive spans alternate between services (RFC 0016 §6.4).
	for _, span := range found {
		dbmodel.FromRow(span.row).ResourceSpans().MoveAndAppendTo(chunk.Results.ResourceSpans())
	}
	return chunk, nil
}

// buildFindSpansQuery selects the span rows the query matches, from the cursor's key onward
// when there is one, ordered by spanKey. It fetches one row beyond the page, which tells
// whether another page exists, and one more per hash the cursor lists, since those rows are
// read again and dropped.
func (r *Reader) buildFindSpansQuery(
	ctx context.Context,
	query tracestore.SpanQueryParams,
	after *spanCursor,
	pageSize uint32,
) (string, []any, error) {
	var q strings.Builder
	q.WriteString(strings.TrimRight(sql.SelectSpansQuery, "\n"))
	q.WriteString("\nWHERE 1=1")
	var args []any
	// Positional time.Time arguments are truncated to seconds by clickhouse-go.
	if !query.StartTimeMin.IsZero() {
		appendAnd(&q, "s.start_time >= fromUnixTimestamp64Nano(?)")
		args = append(args, query.StartTimeMin.UnixNano())
	}
	if !query.StartTimeMax.IsZero() {
		appendAnd(&q, "s.start_time <= fromUnixTimestamp64Nano(?)")
		args = append(args, query.StartTimeMax.UnixNano())
	}
	if query.Filter != nil {
		var err error
		if args, err = r.appendFilter(ctx, &q, args, query.Filter); err != nil {
			return "", nil, err
		}
	}
	fetch := uint64(pageSize) + 1
	if after != nil {
		// The continuation is inclusive of the cursor's key, so that the rows tying on it are
		// read again and the cursor's hashes decide which of them the earlier pages hold.
		appendAnd(&q, "(s.start_time < fromUnixTimestamp64Nano(?) OR (s.start_time = fromUnixTimestamp64Nano(?) AND (s.trace_id > ? OR (s.trace_id = ? AND s.id >= ?))))")
		traceID := after.key.traceID.String()
		args = append(args, after.key.startTime, after.key.startTime, traceID, traceID, after.key.spanID.String())
		fetch += uint64(len(after.returned))
	}
	q.WriteString("\nORDER BY s.start_time DESC, s.trace_id ASC, s.id ASC")
	q.WriteString("\nLIMIT ?")
	args = append(args, fetch)
	return q.String(), args, nil
}

// readFoundSpans scans rows into foundSpans until it holds want of them, dropping the rows at
// the cursor's key that the cursor says earlier pages returned, one row per listed hash. It
// stops reading at want rather than trust the statement's LIMIT, which a token sized.
func readFoundSpans(rows driver.Rows, after *spanCursor, want uint32) ([]foundSpan, error) {
	var returned []uint64
	if after != nil {
		returned = slices.Clone(after.returned)
	}
	var found []foundSpan
	err := readRows(rows, "span", func(rows driver.Rows) error {
		row, err := dbmodel.ScanRow(rows)
		if err != nil {
			return fmt.Errorf("failed to scan span row: %w", err)
		}
		span, err := newFoundSpan(row)
		if err != nil {
			return err
		}
		if after != nil && span.key == after.key {
			if i := slices.Index(returned, span.hash()); i >= 0 {
				returned = slices.Delete(returned, i, i+1)
				return nil
			}
		}
		found = append(found, span)
		if uint64(len(found)) == uint64(want) {
			return errStopReading
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopReading) {
		return nil, err
	}
	return found, nil
}

func newFoundSpan(row *dbmodel.SpanRow) (foundSpan, error) {
	traceID, err := jptrace.TraceIDFromString(row.TraceID)
	if err != nil {
		return foundSpan{}, fmt.Errorf("failed to decode trace ID: %w", err)
	}
	spanID, err := jptrace.SpanIDFromString(row.ID)
	if err != nil {
		return foundSpan{}, fmt.Errorf("failed to decode span ID: %w", err)
	}
	return foundSpan{
		key: spanKey{
			startTime: row.StartTime.UnixNano(),
			traceID:   traceID,
			spanID:    spanID,
		},
		row: row,
	}, nil
}

// nextSpanCursor is the cursor after a full page: the key of its last span, and the hashes of
// every span in the page with that key, plus those the incoming cursor listed when the page
// never left its key.
func nextSpanCursor(page []foundSpan, after *spanCursor) spanCursor {
	next := spanCursor{key: page[len(page)-1].key}
	for i := len(page) - 1; i >= 0 && page[i].key == next.key; i-- {
		next.returned = append(next.returned, page[i].hash())
	}
	if after != nil && after.key == next.key {
		next.returned = append(next.returned, after.returned...)
	}
	return next
}
