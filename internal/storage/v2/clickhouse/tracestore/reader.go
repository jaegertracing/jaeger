// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ tracestore.Reader = (*Reader)(nil)

// changed: Reader now supports native trace summaries
var _ tracestore.SummaryReader = (*Reader)(nil)

type ReaderConfig struct {
	// DefaultSearchDepth is the default number of trace IDs to return when searching for traces.
	// This value is used when the SearchDepth field in TraceQueryParams is not set.
	DefaultSearchDepth int
	// MaxSearchDepth is the maximum number of trace IDs that can be returned when searching for traces.
	// This value is used to limit the SearchDepth field in TraceQueryParams.
	MaxSearchDepth int
	// AttributeMetadataCacheTTL is the time-to-live for cached attribute metadata entries.
	AttributeMetadataCacheTTL time.Duration
	// AttributeMetadataCacheMaxSize is the maximum number of entries in the attribute metadata cache.
	AttributeMetadataCacheMaxSize int
}

type Reader struct {
	conn          driver.Conn
	config        ReaderConfig
	attrMetaCache cache.Cache
}

// NewReader returns a new Reader instance that uses the given ClickHouse connection
// to read trace data.
//
// The provided connection is used exclusively for reading traces, meaning it is safe
// to enable instrumentation on the connection without risk of recursively generating traces.
func NewReader(conn driver.Conn, cfg ReaderConfig) *Reader {
	attrMetaCache := cache.NewLRUWithOptions(cfg.AttributeMetadataCacheMaxSize, &cache.Options{
		TTL: cfg.AttributeMetadataCacheTTL,
	})
	return &Reader{conn: conn, config: cfg, attrMetaCache: attrMetaCache}
}

func (r *Reader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, traceID := range traceIDs {
			query, args := buildGetTracesQuery(traceID)
			rows, err := r.conn.Query(ctx, query, args...)
			if err != nil {
				yield(nil, fmt.Errorf("failed to query trace: %w", err))
				return
			}

			var errs []error
			for rows.Next() {
				span, scanErr := dbmodel.ScanRow(rows)
				if scanErr != nil {
					errs = append(errs, fmt.Errorf("failed to scan span row: %w", scanErr))
					break
				}
				trace := dbmodel.FromRow(span)
				if !yield([]ptrace.Traces{trace}, nil) {
					_ = rows.Close()
					return
				}
			}
			if rowsErr := rows.Err(); rowsErr != nil {
				errs = append(errs, fmt.Errorf("failed to read span rows: %w", rowsErr))
			}
			if closeErr := rows.Close(); closeErr != nil {
				errs = append(errs, fmt.Errorf("failed to close rows: %w", closeErr))
			}
			if err := errors.Join(errs...); err != nil {
				yield(nil, err)
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

	var (
		services []string
		errs     []error
	)
	for rows.Next() {
		var service dbmodel.Service
		if scanErr := rows.ScanStruct(&service); scanErr != nil {
			errs = append(errs, fmt.Errorf("failed to scan row: %w", scanErr))
			break
		}
		services = append(services, service.Name)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		errs = append(errs, fmt.Errorf("failed to read service rows: %w", rowsErr))
	}
	if closeErr := rows.Close(); closeErr != nil {
		errs = append(errs, fmt.Errorf("failed to close rows: %w", closeErr))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
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

	var (
		operations []tracestore.Operation
		errs       []error
	)
	for rows.Next() {
		var operation dbmodel.Operation
		if scanErr := rows.ScanStruct(&operation); scanErr != nil {
			errs = append(errs, fmt.Errorf("failed to scan row: %w", scanErr))
			break
		}
		operations = append(operations, tracestore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		errs = append(errs, fmt.Errorf("failed to read operation rows: %w", rowsErr))
	}
	if closeErr := rows.Close(); closeErr != nil {
		errs = append(errs, fmt.Errorf("failed to close rows: %w", closeErr))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return operations, nil
}

func (r *Reader) FindTraces(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		traceIDsQuery, args, err := r.buildFindTraceIDsQuery(ctx, query)
		if err != nil {
			yield(nil, fmt.Errorf("failed to build query: %w", err))
			return
		}

		rows, err := r.conn.Query(ctx, buildFindTracesQuery(traceIDsQuery), args...)
		if err != nil {
			yield(nil, fmt.Errorf("failed to query traces: %w", err))
			return
		}

		var errs []error
		for rows.Next() {
			span, scanErr := dbmodel.ScanRow(rows)
			if scanErr != nil {
				errs = append(errs, fmt.Errorf("failed to scan span row: %w", scanErr))
				break
			}
			trace := dbmodel.FromRow(span)
			if !yield([]ptrace.Traces{trace}, nil) {
				_ = rows.Close()
				return
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			errs = append(errs, fmt.Errorf("failed to read span rows: %w", rowsErr))
		}
		if closeErr := rows.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close rows: %w", closeErr))
		}
		if err := errors.Join(errs...); err != nil {
			yield(nil, err)
		}
	}
}

// changed: native summary query without materializing full traces
func (r *Reader) FindTraceSummaries(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		traceIDsQuery, args, err := r.buildFindTraceIDsQuery(ctx, query)
		if err != nil {
			yield(nil, fmt.Errorf("failed to build query: %w", err))
			return
		}

		querySQL := fmt.Sprintf(`
			SELECT
				trace_id,
				min(timestamp) AS min_start_time,
				max(timestamp + duration) AS max_end_time,
				count() AS span_count,
				countIf(status_code = 'STATUS_CODE_ERROR') AS error_span_count,
				argMin(service_name, timestamp) AS root_service_name,
				argMin(span_name, timestamp) AS root_operation_name
			FROM spans
			WHERE trace_id IN (%s)
			GROUP BY trace_id
			ORDER BY min_start_time DESC
		`, traceIDsQuery)

		rows, err := r.conn.Query(ctx, querySQL, args...)
		if err != nil {
			yield(nil, fmt.Errorf("failed to query trace summaries: %w", err))
			return
		}

		var (
			summaries []tracestore.TraceSummary
			errs      []error
		)

		for rows.Next() {
			var (
				traceIDHex string
				summary    tracestore.TraceSummary
			)

			if scanErr := rows.Scan(
				&traceIDHex,
				&summary.MinStartTime,
				&summary.MaxEndTime,
				&summary.SpanCount,
				&summary.ErrorSpanCount,
				&summary.RootServiceName,
				&summary.RootOperationName,
			); scanErr != nil {
				errs = append(errs, fmt.Errorf("failed to scan summary row: %w", scanErr))
				break
			}

			traceIDBytes, decodeErr := hex.DecodeString(traceIDHex)
			if decodeErr != nil {
				errs = append(errs, fmt.Errorf("failed to decode trace ID: %w", decodeErr))
				break
			}

			summary.TraceID = pcommon.TraceID(traceIDBytes)

			// changed: avoid full trace reconstruction for orphan spans
			summary.OrphanSpanCount = 0

			summaries = append(summaries, summary)
		}

		if rowsErr := rows.Err(); rowsErr != nil {
			errs = append(errs, fmt.Errorf("failed to read summary rows: %w", rowsErr))
		}
		if closeErr := rows.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close rows: %w", closeErr))
		}
		if err := errors.Join(errs...); err != nil {
			yield(nil, err)
			return
		}

		yield(summaries, nil)
	}
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
		q, args, err := r.buildFindTraceIDsQuery(ctx, query)
		if err != nil {
			yield(nil, fmt.Errorf("failed to build query: %w", err))
			return
		}

		rows, err := r.conn.Query(ctx, q, args...)
		if err != nil {
			yield(nil, fmt.Errorf("failed to query trace IDs: %w", err))
			return
		}

		var errs []error
		for rows.Next() {
			traceID, scanErr := readRowIntoTraceID(rows)
			if scanErr != nil {
				errs = append(errs, scanErr)
				break
			}
			if !yield(traceID, nil) {
				_ = rows.Close()
				return
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			errs = append(errs, fmt.Errorf("failed to read trace ID rows: %w", rowsErr))
		}
		if closeErr := rows.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close rows: %w", closeErr))
		}
		if err := errors.Join(errs...); err != nil {
			yield(nil, err)
		}
	}
}