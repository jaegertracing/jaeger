// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/xpdata"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var _ tracestore.Reader = (*Reader)(nil)

var errAttributeMetadataNotFound = fmt.Errorf("attribute metadata not found for key")

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

func (r *Reader) FindTraces(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		traceIDsQuery, args, err := r.buildFindTraceIDsQuery(query)
		if err != nil {
			yield(nil, fmt.Errorf("failed to build query: %w", err))
			return
		}

		rows, err := r.conn.Query(ctx, buildFindTracesQuery(traceIDsQuery), args...)
		if err != nil {
			yield(nil, fmt.Errorf("failed to query traces: %w", err))
			return
		}
		defer rows.Close()

		for rows.Next() {
			span, err := dbmodel.ScanRow(rows)
			if err != nil {
				if !yield(nil, fmt.Errorf("failed to scan span row: %w", err)) {
					break
				}
				continue
			}
			trace := dbmodel.FromRow(span)
			if !yield([]ptrace.Traces{trace}, nil) {
				break
			}
		}
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
		q, args, err := r.buildFindTraceIDsQuery(query)
		if errors.Is(err, errAttributeMetadataNotFound) {
			return
		} else if err != nil {
			yield(nil, fmt.Errorf("failed to build query: %w", err))
			return
		}

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

// attributeMetadata maps attribute keys to levels, and each level to a list of types.
// Structure: attributeMetadata[key][level] = []type
// Example: attributeMetadata["http.status"]["span"] = ["int", "str"]
type attributeMetadata map[string]map[string][]string

// getAttributeMetadata retrieves the types stored in ClickHouse for string attributes.
//
// The query service forwards all attribute filters as strings (via AsString()), regardless
// of their actual type. For example:
//   - A bool attribute stored as true becomes the string "true"
//   - An int attribute stored as 123 becomes the string "123"
//
// To query ClickHouse correctly, we need to:
//  1. Look up the actual type(s) from the attribute_metadata table
//  2. Convert the string back to the original type
//  3. Query the appropriate typed column (bool_attributes, int_attributes, etc.)
//
// Since attributes can be stored with different types across different spans
// (e.g. "http.status" could be an int in one span and a string in another),
// the metadata can return multiple types for a single key. We build OR conditions
// to match any of the possible types.
//
// Only string-typed attributes from the query are looked up, since other types
// (bool, int, double, etc.) are already correctly typed in the query parameters.
func (r *Reader) getAttributeMetadata(attributes pcommon.Map) (attributeMetadata, error) {
	query, args := buildSelectAttributeMetadataQuery(attributes)
	rows, err := r.conn.Query(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query attribute metadata: %w", err)
	}
	defer rows.Close()

	metadata := make(attributeMetadata)
	for rows.Next() {
		var attrMeta dbmodel.AttributeMetadata
		if err := rows.ScanStruct(&attrMeta); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if metadata[attrMeta.AttributeKey] == nil {
			metadata[attrMeta.AttributeKey] = make(map[string][]string)
		}
		metadata[attrMeta.AttributeKey][attrMeta.Level] = append(metadata[attrMeta.AttributeKey][attrMeta.Level], attrMeta.Type)
	}
	return metadata, nil
}

// marshalValueForQuery is a small test seam to allow injecting marshal errors
// for complex attributes in unit tests. In production it uses xpdata.JSONMarshaler.
var marshalValueForQuery = func(v pcommon.Value) (string, error) {
	m := &xpdata.JSONMarshaler{}
	b, err := m.MarshalValue(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func buildFindTracesQuery(traceIDsQuery string) string {
	return sql.SelectSpansQuery + " WHERE s.trace_id IN (SELECT trace_id FROM (" + traceIDsQuery + ")) ORDER BY s.trace_id"
}

func buildSelectAttributeMetadataQuery(attributes pcommon.Map) (string, []any) {
	var q strings.Builder
	q.WriteString(sql.SelectAttributeMetadata)
	args := []any{}

	var placeholders []string
	for key, attr := range attributes.All() {
		if attr.Type() == pcommon.ValueTypeStr {
			placeholders = append(placeholders, "?")
			args = append(args, key)
		}
	}

	if len(placeholders) > 0 {
		q.WriteString(" WHERE attribute_key IN (")
		q.WriteString(strings.Join(placeholders, ", "))
		q.WriteString(")")
	}
	q.WriteString(" GROUP BY attribute_key, type, level")
	fmt.Println(q.String())
	return q.String(), args
}

func (r *Reader) buildFindTraceIDsQuery(
	query tracestore.TraceQueryParams,
) (string, []any, error) {
	limit := query.SearchDepth
	if limit == 0 {
		limit = r.config.DefaultSearchDepth
	}
	if limit > r.config.MaxSearchDepth {
		return "", nil, fmt.Errorf("search depth %d exceeds maximum allowed %d", limit, r.config.MaxSearchDepth)
	}

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

	attributeMetadata, err := r.getAttributeMetadata(query.Attributes)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get attribute metadata: %w", err)
	}
	if err := r.buildAttributeConditions(&q, &args, query.Attributes, attributeMetadata); err != nil {
		return "", nil, err
	}

	q.WriteString(" LIMIT ?")
	args = append(args, limit)

	return q.String(), args, nil
}

func (r *Reader) buildAttributeConditions(q *strings.Builder, args *[]any, attributes pcommon.Map, metadata attributeMetadata) error {
	for key, attr := range attributes.All() {
		q.WriteString(" AND (")

		switch attr.Type() {
		case pcommon.ValueTypeBool:
			r.buildBoolAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeDouble:
			r.buildDoubleAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeInt:
			r.buildIntAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeStr:
			if err := r.buildStringAttributeCondition(q, args, key, attr, metadata); err != nil {
				return err
			}
		case pcommon.ValueTypeBytes:
			r.buildBytesAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeSlice:
			if err := r.buildSliceAttributeCondition(q, args, key, attr); err != nil {
				return err
			}
		case pcommon.ValueTypeMap:
			if err := r.buildMapAttributeCondition(q, args, key, attr); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported attribute type %v for key %s", attr.Type(), key)
		}

		q.WriteString(")")
	}

	return nil
}

func (r *Reader) buildBoolAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.bool_attributes.key, s.bool_attributes.value)")
	q.WriteString(" OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_bool_attributes.key, s.resource_bool_attributes.value)")
	*args = append(*args, key, attr.Bool(), key, attr.Bool())
}

func (r *Reader) buildDoubleAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.double_attributes.key, s.double_attributes.value)")
	q.WriteString(" OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_double_attributes.key, s.resource_double_attributes.value)")
	*args = append(*args, key, attr.Double(), key, attr.Double())
}

func (r *Reader) buildIntAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)")
	q.WriteString(" OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)")
	*args = append(*args, key, attr.Int(), key, attr.Int())
}

func (r *Reader) buildBytesAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	attrKey := "@bytes@" + key
	val := base64.StdEncoding.EncodeToString(attr.Bytes().AsRaw())
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)")
	q.WriteString(" OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)")
	*args = append(*args, attrKey, val, attrKey, val)
}

func (r *Reader) buildSliceAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@slice@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal slice attribute %q: %w", key, err)
	}
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)")
	q.WriteString(" OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)")
	*args = append(*args, attrKey, b, attrKey, b)
	return nil
}

func (r *Reader) buildMapAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@map@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal map attribute %q: %w", key, err)
	}
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)")
	q.WriteString(" OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)")
	*args = append(*args, attrKey, b, attrKey, b)
	return nil
}

// buildStringAttributeCondition adds a condition for string attributes by looking up their
// actual stored type(s) and level(s) from the attribute_metadata table.
//
// String attributes require special handling because the query service passes all
// attributes as strings (via AsString()), regardless of their actual stored type.
// We must look up the attribute_metadata to determine the actual type(s) and
// level(s) where this attribute is stored, then convert the string back to the
// appropriate type for querying.
func (r *Reader) buildStringAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value, metadata attributeMetadata) error {
	levelTypes, ok := metadata[key]
	if !ok {
		return errAttributeMetadataNotFound
	}

	first := true
	for level, types := range levelTypes {
		for _, t := range types {
			if !first {
				q.WriteString(" OR ")
			}
			first = false

			attrKey := key
			var val any

			switch t {
			case "bool":
				if b, err := strconv.ParseBool(attr.Str()); err == nil {
					val = b
				} else {
					return fmt.Errorf("failed to parse bool attribute %q: %w", key, err)
				}
			case "double":
				if f, err := strconv.ParseFloat(attr.Str(), 64); err == nil {
					val = f
				} else {
					return fmt.Errorf("failed to parse double attribute %q: %w", key, err)
				}
			case "int":
				if i, err := strconv.ParseInt(attr.Str(), 10, 64); err == nil {
					val = i
				} else {
					return fmt.Errorf("failed to parse int attribute %q: %w", key, err)
				}
			case "str":
				val = attr.Str()
			case "bytes":
				attrKey = "@bytes@" + key
				decoded, err := base64.StdEncoding.DecodeString(attr.Str())
				if err != nil {
					return fmt.Errorf("failed to decode bytes attribute %q: %w", key, err)
				}
				val = string(decoded)
			case "map":
				attrKey = "@map@" + key
				val = attr.Str()
			case "slice":
				attrKey = "@slice@" + key
				val = attr.Str()
			// TODO: support map and slice
			default:
				return fmt.Errorf("unsupported attribute type %q for key %q", t, key)
			}

			var colType string
			if t == "bytes" || t == "map" || t == "slice" {
				colType = "complex"
			} else {
				colType = t
			}

			var prefix string
			if level == "resource" {
				prefix = "resource_"
			} else if level == "scope" {
				prefix = "scope_"
			} else {
				prefix = ""
			}

			q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + prefix + colType + "_attributes.key, s." + prefix + colType + "_attributes.value)")
			*args = append(*args, attrKey, val)
		}
	}

	return nil
}
