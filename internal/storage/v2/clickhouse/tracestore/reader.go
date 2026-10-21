// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ tracestore.Reader = (*Reader)(nil)

// traceSearchChunkSize caps the number of trace IDs per bounds lookup and span
// fetch statement. clickhouse-go inlines the arguments into the SQL text, which
// the server parses under max_query_size (256 KiB by default).
const traceSearchChunkSize = 500

type ReaderConfig struct {
	// DefaultSearchDepth is the default number of trace IDs to return when searching for traces.
	// This value is used when the SearchDepth field in TraceQueryParams is not set.
	DefaultSearchDepth uint32
	// MaxSearchDepth is the maximum number of trace IDs that can be returned when searching for traces.
	// This value is used to limit the SearchDepth field in TraceQueryParams.
	MaxSearchDepth uint32
	// AttributeMetadataCacheTTL is the time-to-live for cached attribute metadata entries.
	AttributeMetadataCacheTTL time.Duration
	// AttributeMetadataCacheMaxSize is the maximum number of entries in the attribute metadata cache.
	AttributeMetadataCacheMaxSize int
}

type Reader struct {
	// ClickHouse does not compute trace summaries natively yet; fall back to
	// FindTraces + client-side aggregation.
	tracestore.UnsupportedTraceSummaries
	// SpanSearch is unsupported in ClickHouse for now.
	tracestore.UnsupportedSpanSearch

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

func (*Reader) SearchCapabilities(context.Context) (tracestore.SearchCapabilities, error) {
	filter := FilterCapabilities()
	return tracestore.SearchCapabilities{
		// The search SQL starts from "WHERE 1=1" and appends the service predicate only
		// when the query carries a name, so an omitted name matches spans from every
		// service.
		WithoutServiceName: true,
		// The search SQL matches span rows, and a conjunction's clauses all apply to the
		// same row, so a conjunction is satisfied within one span rather than across a
		// trace.
		SameSpanConjunction: true,
		Filter:              &filter,
	}, nil
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
		traceIDs, err := r.findTraceIDsWithBounds(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		if len(traceIDs) == 0 {
			return
		}

		for chunk := range slices.Chunk(traceIDs, traceSearchChunkSize) {
			if !r.findTracesChunk(ctx, chunk, yield) {
				return
			}
		}
	}
}

// findTracesChunk fetches the spans of one chunk of traces and reports whether
// the caller should continue with the next chunk.
func (r *Reader) findTracesChunk(
	ctx context.Context,
	traceIDs []tracestore.FoundTraceID,
	yield func([]ptrace.Traces, error) bool,
) bool {
	q, args := buildFindTracesQuery(traceIDs)
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		yield(nil, fmt.Errorf("failed to query traces: %w", err))
		return false
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
			return false
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
		return false
	}
	return true
}

// searchTraceIDs materializes the bounded candidate set before querying any
// Distributed lookup table, avoiding nested distributed subqueries and joins.
func (r *Reader) searchTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) ([]tracestore.FoundTraceID, error) {
	q, args, err := r.buildFindTraceIDsQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query trace IDs: %w", err)
	}
	var traceIDs []tracestore.FoundTraceID
	var errs []error
	for rows.Next() {
		var hex string
		if err := rows.Scan(&hex); err != nil {
			errs = append(errs, fmt.Errorf("failed to scan row: %w", err))
			break
		}
		id, err := jptrace.TraceIDFromString(hex)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to decode trace ID: %w", err))
			break
		}
		traceIDs = append(traceIDs, tracestore.FoundTraceID{TraceID: id})
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Errorf("failed to read trace ID rows: %w", err))
	}
	if err := rows.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close rows: %w", err))
	}
	return traceIDs, errors.Join(errs...)
}

func (r *Reader) findTraceIDsWithBounds(ctx context.Context, query tracestore.TraceQueryParams) ([]tracestore.FoundTraceID, error) {
	traceIDs, err := r.searchTraceIDs(ctx, query)
	if err != nil || len(traceIDs) == 0 {
		return nil, err
	}
	positions := make(map[pcommon.TraceID]int, len(traceIDs))
	for i, trace := range traceIDs {
		positions[trace.TraceID] = i
	}
	for chunk := range slices.Chunk(traceIDs, traceSearchChunkSize) {
		ids := make([]string, 0, len(chunk))
		for _, trace := range chunk {
			ids = append(ids, trace.TraceID.String())
		}
		if err := r.readTraceBounds(ctx, ids, traceIDs, positions); err != nil {
			return nil, err
		}
	}
	return traceIDs, nil
}

// readTraceBounds looks up the bounds of the given IDs and stores them into
// traceIDs at the index recorded in positions.
func (r *Reader) readTraceBounds(
	ctx context.Context,
	ids []string,
	traceIDs []tracestore.FoundTraceID,
	positions map[pcommon.TraceID]int,
) error {
	rows, err := r.conn.Query(ctx, sql.SelectTraceIDTimestamps, ids)
	if err != nil {
		return fmt.Errorf("failed to query trace timestamps: %w", err)
	}
	var errs []error
	for rows.Next() {
		trace, err := readRowIntoTraceID(rows)
		if err != nil {
			errs = append(errs, err)
			break
		}
		if i, ok := positions[trace.TraceID]; ok {
			traceIDs[i] = trace
		}
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Errorf("failed to read trace timestamp rows: %w", err))
	}
	if err := rows.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close rows: %w", err))
	}
	return errors.Join(errs...)
}

func readRowIntoTraceID(rows driver.Rows) (tracestore.FoundTraceID, error) {
	var traceIDHex string
	var start, end time.Time

	if err := rows.Scan(&traceIDHex, &start, &end); err != nil {
		return tracestore.FoundTraceID{}, fmt.Errorf("failed to scan row: %w", err)
	}

	id, err := jptrace.TraceIDFromString(traceIDHex)
	if err != nil {
		return tracestore.FoundTraceID{}, fmt.Errorf("failed to decode trace ID: %w", err)
	}

	traceID := tracestore.FoundTraceID{
		TraceID: id,
	}

	if !start.IsZero() {
		traceID.Start = start
	}
	if !end.IsZero() {
		traceID.End = end
	}

	return traceID, nil
}

func (r *Reader) FindTraceIDs(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[tracestore.PageChunk[[]tracestore.FoundTraceID], error] {
	return func(yield func(tracestore.PageChunk[[]tracestore.FoundTraceID], error) bool) {
		traceIDs, err := r.findTraceIDsWithBounds(ctx, query)
		if err != nil {
			yield(tracestore.PageChunk[[]tracestore.FoundTraceID]{}, err)
			return
		}

		// TODO: Populate NextPageToken when ClickHouse supports RFC 0014 pagination.
		for _, trace := range traceIDs {
			if !yield(tracestore.PageChunk[[]tracestore.FoundTraceID]{Results: []tracestore.FoundTraceID{trace}}, nil) {
				return
			}
		}
	}
}
