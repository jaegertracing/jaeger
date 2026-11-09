// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// defaultSpanOrder is the ORDER BY of the default span order of RFC 0016 §6.1: start time
// descending, then the trace ID and the span ID ascending as tie-breakers. The IDs are
// fixed-width lower-case hex strings, so their bytewise order is their numeric order.
const defaultSpanOrder = "s.start_time DESC, s.trace_id ASC, s.id ASC"

// FindSpans returns the spans matching query, in the query's effective order, cut at the page
// size. It selects the span rows directly, with the same predicates the trace search assembles
// and without the trace-ID subquery, in the default span order. See RFC 0016 §6.
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
	// must refuse it too while SpanSorting is off.
	capabilities, _ := r.SearchCapabilities(ctx)
	if err := capabilities.ValidateSpanSorting(query.OrderBy); err != nil {
		return none, err
	}
	// The query service clamps the page size; a caller reaching the reader directly gets the
	// same bound (RFC 0014 §4).
	pageSize := min(query.Pagination.PageSize, tracestore.MaxPageSize)
	if pageSize == 0 {
		return none, fmt.Errorf("%w: page size must be greater than 0", tracestore.ErrPaginationInvalid)
	}
	q, args, err := r.buildFindSpansQuery(ctx, query, pageSize)
	if err != nil {
		return none, fmt.Errorf("failed to build query: %w", err)
	}
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return none, fmt.Errorf("failed to query spans: %w", err)
	}
	found, err := readSpanRows(rows, pageSize)
	if err != nil {
		return none, err
	}
	chunk := tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces()}
	// Every span keeps its own ResourceSpans, so the page keeps the SQL order even where
	// consecutive spans alternate between services.
	for _, row := range found {
		dbmodel.FromRow(row).ResourceSpans().MoveAndAppendTo(chunk.Results.ResourceSpans())
	}
	return chunk, nil
}

// buildFindSpansQuery selects the span rows the query matches, in the default span order and
// cut at the page size.
func (r *Reader) buildFindSpansQuery(
	ctx context.Context,
	query tracestore.SpanQueryParams,
	pageSize uint32,
) (string, []any, error) {
	var q sqlBuilder
	q.WriteString(strings.TrimRight(sql.SelectSpansQuery, "\n"))
	q.WriteString("\nWHERE 1=1")
	args := q.appendTimeRange(nil, query.StartTimeMin, query.StartTimeMax)
	if query.Filter != nil {
		var err error
		if args, err = r.appendFilter(ctx, &q, args, query.Filter); err != nil {
			return "", nil, err
		}
	}
	q.WriteString("\nORDER BY " + defaultSpanOrder)
	q.WriteString("\nLIMIT ?")
	args = append(args, uint64(pageSize))
	return q.String(), args, nil
}

// readSpanRows scans rows into span rows until it holds want of them. It stops reading at want
// rather than trust the statement's LIMIT.
func readSpanRows(rows driver.Rows, want uint32) ([]*dbmodel.SpanRow, error) {
	var found []*dbmodel.SpanRow
	err := readRows(rows, "span", func(rows driver.Rows) error {
		row, err := dbmodel.ScanRow(rows)
		if err != nil {
			return fmt.Errorf("failed to scan span row: %w", err)
		}
		found = append(found, row)
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
