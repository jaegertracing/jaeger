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
		bool_attributes.key,
		bool_attributes.value,
		double_attributes.key,
		double_attributes.value,
		int_attributes.key,
		int_attributes.value,
		str_attributes.key,
		str_attributes.value,
		complex_attributes.key,
		complex_attributes.value,
		events.name,
		events.timestamp,
		events.bool_attributes.key,
		events.bool_attributes.value,
		events.double_attributes.key,
		events.double_attributes.value,
		events.int_attributes.key,
		events.int_attributes.value,
		events.str_attributes.key,
		events.str_attributes.value,
		events.complex_attributes.key,
		events.complex_attributes.value,
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

type spanRow struct {
	id                          string
	traceID                     string
	traceState                  string
	parentSpanID                string
	name                        string
	kind                        string
	startTime                   time.Time
	statusCode                  string
	statusMessage               string
	rawDuration                 int64
	boolAttributeKeys           []string
	boolAttributeValues         []bool
	doubleAttributeKeys         []string
	doubleAttributeValues       []float64
	intAttributeKeys            []string
	intAttributeValues          []int64
	strAttributeKeys            []string
	strAttributeValues          []string
	complexAttributeKeys        []string
	complexAttributeValues      []string
	eventNames                  []string
	eventTimestamps             []time.Time
	eventBoolAttributeKeys      [][]string
	eventBoolAttributeValues    [][]bool
	eventDoubleAttributeKeys    [][]string
	eventDoubleAttributeValues  [][]float64
	eventIntAttributeKeys       [][]string
	eventIntAttributeValues     [][]int64
	eventStrAttributeKeys       [][]string
	eventStrAttributeValues     [][]string
	eventComplexAttributeKeys   [][]string
	eventComplexAttributeValues [][]string
	linkTraceIDs                []string
	linkSpanIDs                 []string
	linkTraceStates             []string
	serviceName                 string
	scopeName                   string
	scopeVersion                string
}

func (sr *spanRow) ToDBModel() dbmodel.Span {
	return dbmodel.Span{
		ID:            sr.id,
		TraceID:       sr.traceID,
		TraceState:    sr.traceState,
		ParentSpanID:  sr.parentSpanID,
		Name:          sr.name,
		Kind:          sr.kind,
		StartTime:     sr.startTime,
		StatusCode:    sr.statusCode,
		StatusMessage: sr.statusMessage,
		Duration:      time.Duration(sr.rawDuration),
		Attributes: dbmodel.Attributes{
			BoolAttributes:    zipAttributes(sr.boolAttributeKeys, sr.boolAttributeValues),
			DoubleAttributes:  zipAttributes(sr.doubleAttributeKeys, sr.doubleAttributeValues),
			IntAttributes:     zipAttributes(sr.intAttributeKeys, sr.intAttributeValues),
			StrAttributes:     zipAttributes(sr.strAttributeKeys, sr.strAttributeValues),
			ComplexAttributes: zipAttributes(sr.complexAttributeKeys, sr.complexAttributeValues),
		},
		Events: buildEvents(
			sr.eventNames,
			sr.eventTimestamps,
			sr.eventBoolAttributeKeys, sr.eventBoolAttributeValues,
			sr.eventDoubleAttributeKeys, sr.eventDoubleAttributeValues,
			sr.eventIntAttributeKeys, sr.eventIntAttributeValues,
			sr.eventStrAttributeKeys, sr.eventStrAttributeValues,
			sr.eventComplexAttributeKeys, sr.eventComplexAttributeValues,
		),
		Links:        buildLinks(sr.linkTraceIDs, sr.linkSpanIDs, sr.linkTraceStates),
		ServiceName:  sr.serviceName,
		ScopeName:    sr.scopeName,
		ScopeVersion: sr.scopeVersion,
	}
}

func scanSpanRow(rows driver.Rows) (dbmodel.Span, error) {
	var span spanRow
	err := rows.Scan(
		&span.id,
		&span.traceID,
		&span.traceState,
		&span.parentSpanID,
		&span.name,
		&span.kind,
		&span.startTime,
		&span.statusCode,
		&span.statusMessage,
		&span.rawDuration,
		&span.boolAttributeKeys,
		&span.boolAttributeValues,
		&span.doubleAttributeKeys,
		&span.doubleAttributeValues,
		&span.intAttributeKeys,
		&span.intAttributeValues,
		&span.strAttributeKeys,
		&span.strAttributeValues,
		&span.complexAttributeKeys,
		&span.complexAttributeValues,
		&span.eventNames,
		&span.eventTimestamps,
		&span.eventBoolAttributeKeys,
		&span.eventBoolAttributeValues,
		&span.eventDoubleAttributeKeys,
		&span.eventDoubleAttributeValues,
		&span.eventIntAttributeKeys,
		&span.eventIntAttributeValues,
		&span.eventStrAttributeKeys,
		&span.eventStrAttributeValues,
		&span.eventComplexAttributeKeys,
		&span.eventComplexAttributeValues,
		&span.linkTraceIDs,
		&span.linkSpanIDs,
		&span.linkTraceStates,
		&span.serviceName,
		&span.scopeName,
		&span.scopeVersion,
	)
	if err != nil {
		return dbmodel.Span{}, err
	}
	return span.ToDBModel(), nil
}

func zipAttributes[T any](keys []string, values []T) []dbmodel.Attribute[T] {
	n := len(keys)
	attrs := make([]dbmodel.Attribute[T], n)
	for i := 0; i < n; i++ {
		attrs[i] = dbmodel.Attribute[T]{Key: keys[i], Value: values[i]}
	}
	return attrs
}

func buildEvents(
	names []string,
	timestamps []time.Time,
	boolAttributeKeys [][]string, boolAttributeValues [][]bool,
	doubleAttributeKeys [][]string, doubleAttributeValues [][]float64,
	intAttributeKeys [][]string, intAttributeValues [][]int64,
	strAttributeKeys [][]string, strAttributeValues [][]string,
	complexAttributeKeys [][]string, complexAttributeValues [][]string,
) []dbmodel.Event {
	var events []dbmodel.Event
	for i := 0; i < len(names) && i < len(timestamps); i++ {
		event := dbmodel.Event{
			Name:      names[i],
			Timestamp: timestamps[i],
			Attributes: dbmodel.Attributes{
				BoolAttributes:    zipAttributes(boolAttributeKeys[i], boolAttributeValues[i]),
				DoubleAttributes:  zipAttributes(doubleAttributeKeys[i], doubleAttributeValues[i]),
				IntAttributes:     zipAttributes(intAttributeKeys[i], intAttributeValues[i]),
				StrAttributes:     zipAttributes(strAttributeKeys[i], strAttributeValues[i]),
				ComplexAttributes: zipAttributes(complexAttributeKeys[i], complexAttributeValues[i]),
			},
		}
		events = append(events, event)
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
