// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/hex"
	"fmt"
	"iter"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ tracestore.Reader = (*Reader)(nil)

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

func (r *Reader) FindTraceIDs(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		q := sql.SearchTraceIDs
		args := []any{}

		if query.ServiceName != "" {
			q += " AND service_name = ?"
			args = append(args, query.ServiceName)
		}
		if query.OperationName != "" {
			q += " AND name = ?"
			args = append(args, query.OperationName)
		}
		if query.SearchDepth > 0 {
			q += " LIMIT ?"
			args = append(args, query.SearchDepth)
		}

		rows, err := r.conn.Query(ctx, q, args...)
		if err != nil {
			yield(nil, fmt.Errorf("failed to query trace IDs: %w", err))
			return
		}
		defer rows.Close()

		for rows.Next() {
			var traceID string
			if err := rows.Scan(&traceID); err != nil {
				if !yield(nil, fmt.Errorf("failed to scan row: %w", err)) {
					return
				}
				continue
			}

			b, err := hex.DecodeString(traceID)
			if err != nil {
				if !yield(nil, fmt.Errorf("failed to decode trace ID: %w", err)) {
					return
				}
				continue
			}

			if !yield([]tracestore.FoundTraceID{{TraceID: pcommon.TraceID(b)}}, nil) {
				return
			}
		}

	}
}
