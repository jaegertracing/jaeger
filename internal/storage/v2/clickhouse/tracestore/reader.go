// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"
	"iter"
	"time"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel" 
)

const (
	sqlSelectSpansByTraceID = `
	SELECT
		id,
		trace_id,
		trace_state,
		parent_span_id,
		name,
		kind,
		start_time,
		status_code,
		status_message,
		duration,
		events.name,
		events.timestamp,
		links.trace_id,
		links.span_id,
		links.trace_state,
		service_name,
		scope_name,
		scope_version
	FROM spans
	WHERE
		trace_id = ?`
	sqlSelectAllServices        = `SELECT DISTINCT name FROM services`
	sqlSelectOperationsAllKinds = `SELECT name, span_kind
	FROM operations
	WHERE service_name = ?`
	sqlSelectOperationsByKind = `SELECT name, span_kind
	FROM operations
	WHERE service_name = ? AND span_kind = ?`
)

type Reader struct {
	conn driver.Conn
}

// NewReader returns a new Reader instance that uses the given ClickHouse connection
// to read trace data.
//
// The provided connection is used exclusively for reading traces, meaning it is safe
// to enable instrumentation on the connection without risk of recursively generating traces.
func NewReader(conn driver.Conn) *Reader {
	return &Reader{conn: conn}
}

func (r *Reader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, traceID := range traceIDs {
			rows, err := r.conn.Query(ctx, sqlSelectSpansByTraceID, traceID.TraceID)
			if err != nil {
				yield(nil, fmt.Errorf("failed to query trace: %w", err))
				return
			}

			done := false
			for rows.Next() {
				span, err := scanSpanRow(rows)
				if err != nil {
					if !yield(nil, fmt.Errorf("failed to scan span row: %w", err)) {
						done = true
						break
					}
					continue
				}

				trace := dbmodel.FromDBModel(span)
				if !yield([]ptrace.Traces{trace}, nil) {
					done = true
					break
				}
			}

			if err := rows.Close(); err != nil {
				yield(nil, fmt.Errorf("failed to close rows: %w", err))
				return
			}

			if done {
				return
			}
		}
	}
}

func scanSpanRow(rows driver.Rows) (dbmodel.Span, error) {
	var (
		span            dbmodel.Span
		rawDuration     int64
		eventNames      []string
		eventTimestamps []time.Time
		linkTraceIDs    []string
		linkSpanIDs     []string
		linkTraceStates []string
	)

	err := rows.Scan(
		&span.ID,
		&span.TraceID,
		&span.TraceState,
		&span.ParentSpanID,
		&span.Name,
		&span.Kind,
		&span.StartTime,
		&span.StatusCode,
		&span.StatusMessage,
		&rawDuration,
		&eventNames,
		&eventTimestamps,
		&linkTraceIDs,
		&linkSpanIDs,
		&linkTraceStates,
		&span.ServiceName,
		&span.ScopeName,
		&span.ScopeVersion,
	)
	if err != nil {
		return span, err
	}

	span.Duration = time.Duration(rawDuration)
	span.Events = buildEvents(eventNames, eventTimestamps)
	span.Links = buildLinks(linkTraceIDs, linkSpanIDs, linkTraceStates)
	return span, nil
}

func buildEvents(names []string, timestamps []time.Time) []dbmodel.Event {
	var events []dbmodel.Event
	for i := 0; i < len(names) && i < len(timestamps); i++ {
		events = append(events, dbmodel.Event{
			Name:      names[i],
			Timestamp: timestamps[i],
		})
	}
	return events
}

func buildLinks(traceIDs, spanIDs, states []string) []dbmodel.Link {
	var links []dbmodel.Link
	for i := 0; i < len(traceIDs) && i < len(spanIDs) && i < len(states); i++ {
		links = append(links, dbmodel.Link{
			TraceID:    traceIDs[i],
			SpanID:     spanIDs[i],
			TraceState: states[i],
		})
	}
	return links
}

func (r *Reader) GetServices(ctx context.Context) ([]string, error) {
	rows, err := r.conn.Query(ctx, sqlSelectAllServices)
	if err != nil {
		return nil, fmt.Errorf("failed to query services: %w", err)
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var service dbmodel.Service
		if err := rows.ScanStruct(&service); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		services = append(services, service.Name)
	}
	return services, nil
}

func (r *Reader) GetOperations(
	ctx context.Context,
	query tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	var rows driver.Rows
	var err error
	if query.SpanKind == "" {
		rows, err = r.conn.Query(ctx, sqlSelectOperationsAllKinds, query.ServiceName)
	} else {
		rows, err = r.conn.Query(ctx, sqlSelectOperationsByKind, query.ServiceName, query.SpanKind)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query operations: %w", err)
	}
	defer rows.Close()

	var operations []tracestore.Operation
	for rows.Next() {
		var operation dbmodel.Operation
		if err := rows.ScanStruct(&operation); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		operations = append(operations, tracestore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
	}
	return operations, nil
}


func (r *Reader) FindTraces(
	ctx context.Context,
	params *tracestore.TraceQueryParams,
) ([]ptrace.Traces, error) {
	var (
		conditions []string
		args       []any
	)

	if params.ServiceName != "" {
		conditions = append(conditions, "service_name = ?")
		args = append(args, params.ServiceName)
	}
	if params.OperationName != "" {
		conditions = append(conditions, "name = ?")
		args = append(args, params.OperationName)
	}
	if !params.StartTimeMin.IsZero() {
		conditions = append(conditions, "start_time >= ?")
		args = append(args, params.StartTimeMin)
	}
	if !params.StartTimeMax.IsZero() {
		conditions = append(conditions, "start_time <= ?")
		args = append(args, params.StartTimeMax)
	}
	if params.DurationMin > 0 {
		conditions = append(conditions, "duration >= ?")
		args = append(args, params.DurationMin)
	}
	if params.DurationMax > 0 {
		conditions = append(conditions, "duration <= ?")
		args = append(args, params.DurationMax)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	sql := fmt.Sprintf(`
		SELECT DISTINCT trace_id
		FROM spans
		%s
		ORDER BY start_time DESC
		LIMIT ?`, where)
	args = append(args, params.SearchDepth)

	traceIDRows, err := r.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query trace_ids: %w", err)
	}
	defer traceIDRows.Close()

	var traceIDs []string
	for traceIDRows.Next() {
		var traceID string
		if err := traceIDRows.Scan(&traceID); err != nil {
			return nil, fmt.Errorf("scan trace_id: %w", err)
		}
		traceIDs = append(traceIDs, traceID)
	}

	if len(traceIDs) == 0 {
		return nil, nil
	}

	var results []ptrace.Traces
	for _, traceID := range traceIDs {
		rows, err := r.conn.Query(ctx, sqlSelectSpansByTraceID, traceID)
		if err != nil {
			return nil, fmt.Errorf("query spans: %w", err)
		}

		var spans []dbmodel.Span
		for rows.Next() {
			span, err := scanSpanRow(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan span row: %w", err)
			}
			spans = append(spans, span)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close rows: %w", err)
		}

		trace := dbmodel.FromDBModelBatch(spans)
		results = append(results, trace)
	}

	return results, nil
}
