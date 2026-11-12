// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// spanSortColumn is a column of the spans table that a span search can order by.
type spanSortColumn string

const (
	columnStartTime spanSortColumn = "start_time"
	columnDuration  spanSortColumn = "duration"
	columnTraceID   spanSortColumn = "trace_id"
	columnSpanID    spanSortColumn = "id"
)

// spanSortColumns maps an ordering term's built-in span field to the column that carries it.
var spanSortColumns = map[string]spanSortColumn{
	expression.SpanFieldStartTime: columnStartTime,
	expression.SpanFieldDuration:  columnDuration,
	expression.SpanFieldTraceID:   columnTraceID,
	expression.SpanFieldSpanID:    columnSpanID,
}

// sortTerm is one term of a span search's ORDER BY: a spans table column and its direction.
type sortTerm struct {
	column     spanSortColumn
	descending bool
}

// placeholder is the bound parameter the term's column compares against. A start time binds
// as Unix nanoseconds, because clickhouse-go truncates a time.Time argument to seconds.
func (c sortTerm) placeholder() string {
	if c.column == columnStartTime {
		return "fromUnixTimestamp64Nano(?)"
	}
	return "?"
}

// keyValueType is the type of the term's value in a cursor: a string for an ID, an integer
// for a start time or a duration.
func (c sortTerm) keyValueType() tracestore.SortKeyValueType {
	switch c.column {
	case columnTraceID, columnSpanID:
		return tracestore.SortKeyString
	default:
		return tracestore.SortKeyInteger
	}
}

// value is the row's value for the term, in the form the cursor carries and the placeholder
// binds: an int64 for a numeric column and a string for an ID.
func (c sortTerm) value(row *dbmodel.SpanRow) any {
	switch c.column {
	case columnStartTime:
		return row.StartTime.UnixNano()
	case columnDuration:
		return row.Duration
	case columnTraceID:
		return row.TraceID
	default:
		return row.ID
	}
}

// spanSort lowers the query's effective order to sort terms. The effective order ends in the
// trace ID and the span ID, so two rows tie on every term only when one span was stored twice
// (RFC 0016 §6.4). Ordering by an ID column orders by the ID's numeric value, since the IDs
// are stored as fixed-width lower-case hex strings whose bytewise order is their numeric order.
func spanSort(order []tracestore.SpanSortOrder) ([]tracestore.SpanSortOrder, []sortTerm, error) {
	order, err := tracestore.EffectiveSpanOrder(order)
	if err != nil {
		return nil, nil, err
	}
	sort, err := lowerSpanOrder(order)
	return order, sort, err
}

// lowerSpanOrder maps each term of an effective order to its column. The order arrives
// settled by tracestore.EffectiveSpanOrder, which admits only field refs to the fields
// spanSortColumns maps, so a term of another shape is a programming error.
func lowerSpanOrder(order []tracestore.SpanSortOrder) ([]sortTerm, error) {
	sort := make([]sortTerm, 0, len(order))
	for _, term := range order {
		ref, ok := term.Expression.(*expression.FieldRef)
		if !ok {
			return nil, fmt.Errorf("ordering term is not a field ref: %T", term.Expression)
		}
		column, ok := spanSortColumns[ref.Name]
		if !ok {
			return nil, fmt.Errorf("ordering term names a field without a sort column: %s", ref.Name)
		}
		sort = append(sort, sortTerm{
			column:     column,
			descending: term.Direction == tracestore.SortDescending,
		})
	}
	return sort, nil
}

// FindSpans returns one page of the spans matching query, in the query's effective order, with
// the token that resumes the search after them. It selects the span rows directly, with the
// same predicates the trace search assembles and without the trace-ID subquery, ordered by the
// sort terms and cut at the page size; the cursor inside the token is the last row's sort
// values, and a continuation reads from them onward. See RFC 0016 §6.
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
	// The fingerprint covers the effective order, so a token minted for the omitted default order
	// continues an explicit equivalent of it, and the other way round (RFC 0016 §6.4).
	order, sort, err := spanSort(query.OrderBy)
	if err != nil {
		return none, err
	}
	query.OrderBy = order
	fingerprint, err := query.Fingerprint()
	if err != nil {
		return none, err
	}
	var after spanCursor
	if query.Pagination.PageToken != "" {
		raw, err := query.Pagination.PageToken.Cursor(fingerprint)
		if err != nil {
			return none, err
		}
		if after, err = decodeSpanCursor(raw, sort); err != nil {
			return none, err
		}
	}
	// The query service clamps the page size; a caller reaching the reader directly gets the
	// same bound (RFC 0014 §4).
	pageSize := min(query.Pagination.PageSize, tracestore.MaxPageSize)
	if pageSize == 0 {
		return none, fmt.Errorf("%w: page size must be greater than 0", tracestore.ErrPaginationInvalid)
	}
	q, args, err := r.buildFindSpansQuery(ctx, query, sort, after, pageSize)
	if err != nil {
		return none, fmt.Errorf("failed to build query: %w", err)
	}
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return none, fmt.Errorf("failed to query spans: %w", err)
	}
	// One row beyond the page tells whether another page exists.
	found, err := readSpanRows(rows)
	if err != nil {
		return none, err
	}
	more := uint64(len(found)) > uint64(pageSize)
	if more {
		found = found[:pageSize]
	}
	chunk := tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces()}
	if more {
		next := newSpanCursor(found[len(found)-1], sort)
		if chunk.NextPageToken, err = tracestore.NewPageToken(fingerprint, next.encode()); err != nil {
			return none, err
		}
	}
	// Every span keeps its own ResourceSpans, so the page keeps the SQL order even where
	// consecutive spans alternate between services.
	for _, row := range found {
		dbmodel.FromRow(row).ResourceSpans().MoveAndAppendTo(chunk.Results.ResourceSpans())
	}
	return chunk, nil
}

// buildFindSpansQuery selects the span rows the query matches, after the cursor's key when
// there is one, ordered by the sort terms. It fetches one row beyond the page, which tells
// whether another page exists.
func (r *Reader) buildFindSpansQuery(
	ctx context.Context,
	query tracestore.SpanQueryParams,
	sort []sortTerm,
	after spanCursor,
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
	if after != nil {
		q.appendAnd("")
		args = q.appendContinuation(args, sort, after)
	}
	q.appendOrderBy(sort)
	q.WriteString("\nLIMIT ?")
	args = append(args, uint64(pageSize)+1)
	return q.String(), args, nil
}

// appendOrderBy writes the ORDER BY clause of the sort terms, each column in its direction.
func (q *sqlBuilder) appendOrderBy(sort []sortTerm) {
	q.WriteString("\nORDER BY ")
	for i, term := range sort {
		if i > 0 {
			q.WriteString(", ")
		}
		q.WriteString("s." + string(term.column))
		if term.descending {
			q.WriteString(" DESC")
		} else {
			q.WriteString(" ASC")
		}
	}
}

// appendContinuation writes the predicate that selects the rows after key in the order of the
// sort terms: the first term is past its value, or equal to it and the remaining terms are
// past theirs.
func (q *sqlBuilder) appendContinuation(args []any, sort []sortTerm, key []any) []any {
	term := sort[0]
	past := ">"
	if term.descending {
		past = "<"
	}
	column := "s." + string(term.column)
	if len(sort) == 1 {
		q.WriteString(column + " " + past + " " + term.placeholder())
		return append(args, key[0])
	}
	q.WriteString("(" + column + " " + past + " " + term.placeholder() + " OR (" + column + " = " + term.placeholder() + " AND ")
	args = append(args, key[0], key[0])
	args = q.appendContinuation(args, sort[1:], key[1:])
	q.WriteString("))")
	return args
}

// readSpanRows scans every row of the result set into a span row. The statement's LIMIT bounds
// the result, so the rows are read to the end and the iteration and close errors are checked
// on every page.
func readSpanRows(rows driver.Rows) ([]*dbmodel.SpanRow, error) {
	var found []*dbmodel.SpanRow
	err := readRows(rows, "span", func(rows driver.Rows) error {
		row, err := dbmodel.ScanRow(rows)
		if err != nil {
			return fmt.Errorf("failed to scan span row: %w", err)
		}
		found = append(found, row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
