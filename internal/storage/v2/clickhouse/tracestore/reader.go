// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"
	"iter"
	"time"

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
		bool_attributes,
		double_attributes,
		int_attributes,
		str_attributes,
		complex_attributes,
		events,
		links,
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
		span              dbmodel.Span
		rawDuration       int64
		boolAttributes    []map[string]any
		doubleAttributes  []map[string]any
		intAttributes     []map[string]any
		strAttributes     []map[string]any
		complexAttributes []map[string]any
		events            []map[string]any
		links             []map[string]any
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
		&boolAttributes,
		&doubleAttributes,
		&intAttributes,
		&strAttributes,
		&complexAttributes,
		&events,
		&links,
		&span.ServiceName,
		&span.ScopeName,
		&span.ScopeVersion,
	)
	if err != nil {
		return span, err
	}

	span.Duration = time.Duration(rawDuration)

	span.Attributes.BoolAttributes = convertAttributes[bool](boolAttributes)
	span.Attributes.DoubleAttributes = convertAttributes[float64](doubleAttributes)
	span.Attributes.IntAttributes = convertAttributes[int64](intAttributes)
	span.Attributes.StrAttributes = convertAttributes[string](strAttributes)
	span.Attributes.ComplexAttributes = convertAttributes[string](complexAttributes)

	span.Events = buildEvents(events)
	span.Links = buildLinks(links)
	return span, nil
}

func zipAttributes[T any](keys []string, values []T) []dbmodel.Attribute[T] {
	n := len(keys)
	attrs := make([]dbmodel.Attribute[T], n)
	for i := 0; i < n; i++ {
		attrs[i] = dbmodel.Attribute[T]{Key: keys[i], Value: values[i]}
	}
	return attrs
}

func convertAttributes[T any](storedAttributes []map[string]any) []dbmodel.Attribute[T] {
	var attributes []dbmodel.Attribute[T]
	for _, attr := range storedAttributes {
		attributes = append(attributes, dbmodel.Attribute[T]{
			Key:   attr["key"].(string),
			Value: attr["value"].(T),
		})
	}
	return attributes
}

func buildEvents(storedEvents []map[string]any) []dbmodel.Event {
	var events []dbmodel.Event
	for _, event := range storedEvents {
		events = append(events, dbmodel.Event{
			Name:      event["name"].(string),
			Timestamp: event["timestamp"].(time.Time),
		})
	}
	return events
}

func buildLinks(links []map[string]any) []dbmodel.Link {
	var result []dbmodel.Link
	for _, link := range links {
		result = append(result, dbmodel.Link{
			TraceID:    link["trace_id"].(string),
			SpanID:     link["span_id"].(string),
			TraceState: link["trace_state"].(string),
		})
	}
	return result
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
