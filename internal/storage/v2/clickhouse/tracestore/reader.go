// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/hex"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ tracestore.Reader = (*Reader)(nil)

type ReaderConfig struct {
	// DefaultSearchDepth is the default number of trace IDs to return when searching for traces.
	// This value is used when the SearchDepth field in TraceQueryParams is not set.
	DefaultSearchDepth int
	// MaxSearchDepth is the maximum number of trace IDs that can be returned when searching for traces.
	// This value is used to limit the SearchDepth field in TraceQueryParams.
	MaxSearchDepth int
}

type Reader struct {
	conn   driver.Conn
	config ReaderConfig
}

// NewReader returns a new Reader instance that uses the given ClickHouse connection
// to read trace data.
//
// The provided connection is used exclusively for reading traces, meaning it is safe
// to enable instrumentation on the connection without risk of recursively generating traces.
func NewReader(conn driver.Conn, cfg ReaderConfig) *Reader {
	return &Reader{conn: conn, config: cfg}
}

func (r *Reader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, traceID := range traceIDs {
			rows, err := r.conn.Query(ctx, sql.SelectSpansByTraceID, traceID.TraceID)
			if err != nil {
				yield(nil, fmt.Errorf("failed to query trace: %w", err))
				return
			}

			done := false
			for rows.Next() {
				span, err := dbmodel.ScanRow(rows)
				if err != nil {
					if !yield(nil, fmt.Errorf("failed to scan span row: %w", err)) {
						done = true
						break
					}
					continue
				}

				trace := dbmodel.FromRow(span)
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

func (r *Reader) GetServices(ctx context.Context) ([]string, error) {
	rows, err := r.conn.Query(ctx, sql.SelectServices)
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
		rows, err = r.conn.Query(ctx, sql.SelectOperationsAllKinds, query.ServiceName)
	} else {
		rows, err = r.conn.Query(ctx, sql.SelectOperationsByKind, query.ServiceName, query.SpanKind)
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
		o := tracestore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		}
		operations = append(operations, o)
	}
	return operations, nil
}

func (*Reader) FindTraces(
	context.Context,
	tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	panic("not implemented")
}

func readRowIntoTraceID(rows driver.Rows) ([]tracestore.FoundTraceID, error) {
	var traceIDHex string
	var start, end time.Time

	if err := rows.Scan(&traceIDHex, &start, &end); err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	b, err := hex.DecodeString(traceIDHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode trace ID: %w", err)
	}

	traceID := tracestore.FoundTraceID{
		TraceID: pcommon.TraceID(b),
	}

	if !start.IsZero() {
		traceID.Start = start
	}
	if !end.IsZero() {
		traceID.End = end
	}

	return []tracestore.FoundTraceID{
		traceID,
	}, nil
}

func (r *Reader) FindTraceIDs(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		limit := query.SearchDepth
		if limit == 0 {
			limit = r.config.DefaultSearchDepth
		}
		if limit > r.config.MaxSearchDepth {
			yield(nil, fmt.Errorf("search depth %d exceeds maximum allowed %d", limit, r.config.MaxSearchDepth))
			return
		}

		q, args := buildFindTraceIDsQuery(query, limit)

		rows, err := r.conn.Query(ctx, q, args...)
		if err != nil {
			yield(nil, fmt.Errorf("failed to query trace IDs: %w", err))
			return
		}
		defer rows.Close()

		for rows.Next() {
			traceID, err := readRowIntoTraceID(rows)
			if !yield(traceID, err) {
				return
			}
		}
	}
}

func buildFindTraceIDsQuery(query tracestore.TraceQueryParams, limit int) (string, []any) {
	var q strings.Builder
	q.WriteString(sql.SearchTraceIDs)
	args := []any{}

	if query.ServiceName != "" {
		q.WriteString(" AND s.service_name = ?")
		args = append(args, query.ServiceName)
	}
	if query.OperationName != "" {
		q.WriteString(" AND s.name = ?")
		args = append(args, query.OperationName)
	}
	if query.DurationMin > 0 {
		q.WriteString(" AND s.duration >= ?")
		args = append(args, query.DurationMin.Nanoseconds())
	}
	if query.DurationMax > 0 {
		q.WriteString(" AND s.duration <= ?")
		args = append(args, query.DurationMax.Nanoseconds())
	}
	if !query.StartTimeMin.IsZero() {
		q.WriteString(" AND s.start_time >= ?")
		args = append(args, query.StartTimeMin)
	}
	if !query.StartTimeMax.IsZero() {
		q.WriteString(" AND s.start_time <= ?")
		args = append(args, query.StartTimeMax)
	}

	for key, attr := range query.Attributes.All() {
		var attrType string
		var val any

		switch attr.Type() {
		case pcommon.ValueTypeBool:
			attrType = "bool"
			val = attr.Bool()
		case pcommon.ValueTypeDouble:
			attrType = "double"
			val = attr.Double()
		case pcommon.ValueTypeInt:
			attrType = "int"
			val = attr.Int()
		case pcommon.ValueTypeStr:
			attrType = "str"
			val = attr.Str()
		default:
			continue
		}

		q.WriteString(" AND (")
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + attrType + "_attributes.key, s." + attrType + "_attributes.value)")
		q.WriteString(" OR ")
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_" + attrType + "_attributes.key, s.resource_" + attrType + "_attributes.value)")
		q.WriteString(")")
		args = append(args, key, val, key, val)
	}

	q.WriteString(" LIMIT ?")
	args = append(args, limit)

	return q.String(), args
}
