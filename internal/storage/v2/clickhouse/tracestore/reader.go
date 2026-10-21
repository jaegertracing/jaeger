// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"iter"
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
			if err := readRows(rows, "span", yieldSpanRow(yield)); err != nil {
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
	var services []string
	err = readRows(rows, "service", func(rows driver.Rows) error {
		var service dbmodel.Service
		if err := rows.ScanStruct(&service); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}
		services = append(services, service.Name)
		return nil
	})
	if err != nil {
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
	var operations []tracestore.Operation
	err = readRows(rows, "operation", func(rows driver.Rows) error {
		var operation dbmodel.Operation
		if err := rows.ScanStruct(&operation); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}
		operations = append(operations, tracestore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return operations, nil
}

// FindTraces runs the search in three statements: the candidate trace IDs, their global time
// bounds, and the spans of each trace within its own bounds.
func (r *Reader) FindTraces(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		traceIDs, err := r.findBoundedTraceIDs(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		if len(traceIDs) == 0 {
			return
		}
		q, args := buildFindTracesQuery(traceIDs)
		rows, err := r.conn.Query(ctx, q, args...)
		if err != nil {
			yield(nil, fmt.Errorf("failed to query traces: %w", err))
			return
		}
		if err := readRows(rows, "span", yieldSpanRow(yield)); err != nil {
			yield(nil, err)
		}
	}
}

func (r *Reader) FindTraceIDs(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[tracestore.PageChunk[[]tracestore.FoundTraceID], error] {
	return func(yield func(tracestore.PageChunk[[]tracestore.FoundTraceID], error) bool) {
		traceIDs, err := r.findBoundedTraceIDs(ctx, query)
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

// findBoundedTraceIDs runs the search and attaches each trace's global bounds. FindTraces and
// FindTraceIDs both call it.
func (r *Reader) findBoundedTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) ([]tracestore.FoundTraceID, error) {
	ids, err := r.queryCandidateTraceIDs(ctx, query)
	if err != nil || len(ids) == 0 {
		return ids, err
	}
	return r.attachTraceBounds(ctx, ids)
}

// queryCandidateTraceIDs runs the bounded search statement and returns IDs without bounds. The
// candidates are materialized before any lookup table is queried, so that a Distributed table
// never receives a nested subquery or a join.
func (r *Reader) queryCandidateTraceIDs(ctx context.Context, query tracestore.TraceQueryParams) ([]tracestore.FoundTraceID, error) {
	q, args, err := r.buildFindTraceIDsQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}
	rows, err := r.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query trace IDs: %w", err)
	}
	var traceIDs []tracestore.FoundTraceID
	err = readRows(rows, "trace ID", func(rows driver.Rows) error {
		var hex string
		if err := rows.Scan(&hex); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}
		id, err := jptrace.TraceIDFromString(hex)
		if err != nil {
			return fmt.Errorf("failed to decode trace ID: %w", err)
		}
		traceIDs = append(traceIDs, tracestore.FoundTraceID{TraceID: id})
		return nil
	})
	return traceIDs, err
}

// attachTraceBounds looks up trace_id_timestamps for the given IDs and fills Start and End in
// place. An ID without a row keeps zero bounds.
func (r *Reader) attachTraceBounds(ctx context.Context, traceIDs []tracestore.FoundTraceID) ([]tracestore.FoundTraceID, error) {
	ids := make([]string, 0, len(traceIDs))
	positions := make(map[pcommon.TraceID]int, len(traceIDs))
	for i, trace := range traceIDs {
		ids = append(ids, trace.TraceID.String())
		positions[trace.TraceID] = i
	}
	rows, err := r.conn.Query(ctx, sql.SelectTraceIDTimestamps, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to query trace timestamps: %w", err)
	}
	err = readRows(rows, "trace timestamp", func(rows driver.Rows) error {
		trace, err := readRowIntoTraceID(rows)
		if err != nil {
			return err
		}
		if i, ok := positions[trace.TraceID]; ok {
			traceIDs[i] = trace
		}
		return nil
	})
	return traceIDs, err
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
	return tracestore.FoundTraceID{
		TraceID: id,
		Start:   start,
		End:     end,
	}, nil
}

// errStopReading is returned by a scan function when the consumer of an iterator wants no more
// rows. readRows then closes the result set and reports no error.
var errStopReading = errors.New("stop reading rows")

// readRows passes every row to scan and then closes the result set. The error it returns joins
// the first scan error with the iteration and close errors, and kind names the rows in the
// iteration error, as in "failed to read span rows".
func readRows(rows driver.Rows, kind string, scan func(driver.Rows) error) error {
	var errs []error
	for rows.Next() {
		if err := scan(rows); err != nil {
			if errors.Is(err, errStopReading) {
				_ = rows.Close()
				return nil
			}
			errs = append(errs, err)
			break
		}
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Errorf("failed to read %s rows: %w", kind, err))
	}
	if err := rows.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close rows: %w", err))
	}
	return errors.Join(errs...)
}

// yieldSpanRow scans a span row and hands it to yield as a single-span trace.
func yieldSpanRow(yield func([]ptrace.Traces, error) bool) func(driver.Rows) error {
	return func(rows driver.Rows) error {
		span, err := dbmodel.ScanRow(rows)
		if err != nil {
			return fmt.Errorf("failed to scan span row: %w", err)
		}
		if !yield([]ptrace.Traces{dbmodel.FromRow(span)}, nil) {
			return errStopReading
		}
		return nil
	}
}
