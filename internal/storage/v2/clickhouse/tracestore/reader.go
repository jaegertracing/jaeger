// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/base64"
	"encoding/hex"
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
		defer rows.Close()

		for rows.Next() {
			traceID, err := readRowIntoTraceID(rows)
			if !yield(traceID, err) {
				return
			}
		}
	}
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

func appendNewlineAndIndent(q *strings.Builder, indent int) {
	q.WriteString("\n")
	for i := 0; i < indent; i++ {
		q.WriteString("\t")
	}
}

func indentBlock(s string) string {
	return "\t" + strings.ReplaceAll(s, "\n", "\n\t")
}

func appendAnd(q *strings.Builder, cond string) {
	appendNewlineAndIndent(q, 1)
	q.WriteString("AND ")
	q.WriteString(cond)
}

func buildFindTracesQuery(traceIDsQuery string) string {
	inner := indentBlock("SELECT trace_id FROM (\n" + indentBlock(strings.TrimSpace(traceIDsQuery)) + "\n)")
	base := strings.TrimRight(sql.SelectSpansQuery, "\n")
	return base + "\nWHERE s.trace_id IN (\n" + inner + "\n)\nORDER BY s.trace_id"
}

func (r *Reader) buildFindTraceIDsQuery(
	ctx context.Context,
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
		appendAnd(&q, "s.service_name = ?")
		args = append(args, query.ServiceName)
	}
	if query.OperationName != "" {
		appendAnd(&q, "s.name = ?")
		args = append(args, query.OperationName)
	}
	if query.DurationMin > 0 {
		appendAnd(&q, "s.duration >= ?")
		args = append(args, query.DurationMin.Nanoseconds())
	}
	if query.DurationMax > 0 {
		appendAnd(&q, "s.duration <= ?")
		args = append(args, query.DurationMax.Nanoseconds())
	}
	if !query.StartTimeMin.IsZero() {
		appendAnd(&q, "s.start_time >= ?")
		args = append(args, query.StartTimeMin)
	}
	if !query.StartTimeMax.IsZero() {
		appendAnd(&q, "s.start_time <= ?")
		args = append(args, query.StartTimeMax)
	}

	// Only query attribute metadata if requested and string attributes are present.
	// Non-string attributes (bool/double/int/bytes/slice/map) don't require metadata.
	var attributeMetadata attributeMetadata
	if hasStringAttributes(query.Attributes) {
		am, err := r.getAttributeMetadata(ctx, query.Attributes)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get attribute metadata: %w", err)
		}
		attributeMetadata = am
	}

	if err := buildAttributeConditions(&q, &args, query.Attributes, attributeMetadata); err != nil {
		return "", nil, err
	}

	q.WriteString("\nLIMIT ?")
	args = append(args, limit)

	return q.String(), args, nil
}

// hasStringAttributes returns true if any attribute in the map is of string type.
func hasStringAttributes(attributes pcommon.Map) bool {
	for _, attr := range attributes.All() {
		if attr.Type() == pcommon.ValueTypeStr {
			return true
		}
	}
	return false
}

func buildAttributeConditions(q *strings.Builder, args *[]any, attributes pcommon.Map, metadata attributeMetadata) error {
	for key, attr := range attributes.All() {
		appendAnd(q, "(")

		switch attr.Type() {
		case pcommon.ValueTypeBool:
			buildBoolAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeDouble:
			buildDoubleAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeInt:
			buildIntAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeStr:
			if err := buildStringAttributeCondition(q, args, key, attr, metadata); err != nil {
				return err
			}
		case pcommon.ValueTypeBytes:
			buildBytesAttributeCondition(q, args, key, attr)
		case pcommon.ValueTypeSlice:
			if err := buildSliceAttributeCondition(q, args, key, attr); err != nil {
				return err
			}
		case pcommon.ValueTypeMap:
			if err := buildMapAttributeCondition(q, args, key, attr); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported attribute type %v for key %s", attr.Type(), key)
		}

		appendNewlineAndIndent(q, 1)
		q.WriteString(")")
	}

	return nil
}

func buildBoolAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendNewlineAndIndent(q, 2)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.bool_attributes.key, s.bool_attributes.value)")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_bool_attributes.key, s.resource_bool_attributes.value)")
	*args = append(*args, key, attr.Bool(), key, attr.Bool())
}

func buildDoubleAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendNewlineAndIndent(q, 2)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.double_attributes.key, s.double_attributes.value)")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_double_attributes.key, s.resource_double_attributes.value)")
	*args = append(*args, key, attr.Double(), key, attr.Double())
}

func buildIntAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendNewlineAndIndent(q, 2)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.int_attributes.key, s.int_attributes.value)")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_int_attributes.key, s.resource_int_attributes.value)")
	*args = append(*args, key, attr.Int(), key, attr.Int())
}

func buildBytesAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	attrKey := "@bytes@" + key
	val := base64.StdEncoding.EncodeToString(attr.Bytes().AsRaw())
	appendNewlineAndIndent(q, 2)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)")
	*args = append(*args, attrKey, val, attrKey, val)
}

func buildSliceAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@slice@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal slice attribute %q: %w", key, err)
	}
	appendNewlineAndIndent(q, 2)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_complex_attributes.key, s.resource_complex_attributes.value)")
	*args = append(*args, attrKey, b, attrKey, b)
	return nil
}

func buildMapAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@map@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal map attribute %q: %w", key, err)
	}
	appendNewlineAndIndent(q, 2)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.complex_attributes.key, s.complex_attributes.value)")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
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
func buildStringAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value, metadata attributeMetadata) error {
	levelTypes, ok := metadata[key]

	// if no metadata found, assume string type
	if !ok {
		appendNewlineAndIndent(q, 2)
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)")
		appendNewlineAndIndent(q, 2)
		q.WriteString("OR ")
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.resource_str_attributes.key, s.resource_str_attributes.value)")
		appendNewlineAndIndent(q, 2)
		q.WriteString("OR ")
		q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s.scope_str_attributes.key, s.scope_str_attributes.value)")
		*args = append(*args, key, attr.Str(), key, attr.Str(), key, attr.Str())
		return nil
	}

	first := true
	appendLevel := func(types []string, prefix string) error {
		for _, t := range types {
			if !first {
				appendNewlineAndIndent(q, 2)
				q.WriteString("OR ")
			}
			first = false

			attrKey := key
			var val any

			switch t {
			case "bool":
				b, err := strconv.ParseBool(attr.Str())
				if err != nil {
					return fmt.Errorf("failed to parse bool attribute %q: %w", key, err)
				}
				val = b
			case "double":
				f, err := strconv.ParseFloat(attr.Str(), 64)
				if err != nil {
					return fmt.Errorf("failed to parse double attribute %q: %w", key, err)
				}
				val = f
			case "int":
				i, err := strconv.ParseInt(attr.Str(), 10, 64)
				if err != nil {
					return fmt.Errorf("failed to parse int attribute %q: %w", key, err)
				}
				val = i
			case "str":
				val = attr.Str()
			case "bytes":
				attrKey = "@bytes@" + key
				val = attr.Str()
			// TODO: support map and slice
			default:
				return fmt.Errorf("unsupported attribute type %q for key %q", t, key)
			}

			colType := t
			if t == "bytes" || t == "map" || t == "slice" {
				colType = "complex"
			}

			appendNewlineAndIndent(q, 2)
			q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + prefix + colType + "_attributes.key, s." + prefix + colType + "_attributes.value)")
			*args = append(*args, attrKey, val)
		}
		return nil
	}

	if err := appendLevel(levelTypes.resource, "resource_"); err != nil {
		return err
	}
	if err := appendLevel(levelTypes.scope, "scope_"); err != nil {
		return err
	}
	if err := appendLevel(levelTypes.span, ""); err != nil {
		return err
	}

	return nil
}
