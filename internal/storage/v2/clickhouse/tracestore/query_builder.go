// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/xpdata"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

// marshalValueForQuery is a simpler wrapper around xpdata.JSONMarshaler.
// It can be overridden in tests to simulate marshaling errors.
var marshalValueForQuery = func(v pcommon.Value) (string, error) {
	m := &xpdata.JSONMarshaler{}
	b, err := m.MarshalValue(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type typedAttributeValue struct {
	key       string
	value     any
	valueType pcommon.ValueType
}

// sqlBuilder assembles a statement clause by clause. Its methods write the fragments the
// query builders and the filter lowering share, each on its own indented line.
type sqlBuilder struct {
	strings.Builder
}

func (q *sqlBuilder) appendNewlineAndIndent(indent int) {
	q.WriteString("\n")
	for range indent {
		q.WriteString("\t")
	}
}

// appendAnd writes cond as one more AND clause of a WHERE that already has a predicate.
func (q *sqlBuilder) appendAnd(cond string) {
	q.appendNewlineAndIndent(1)
	q.WriteString("AND ")
	q.WriteString(cond)
}

// appendTimeRange writes the start-time bounds a query carries, skipping a zero bound. The
// bounds bind as Unix nanoseconds, because clickhouse-go truncates a time.Time argument to
// seconds.
func (q *sqlBuilder) appendTimeRange(args []any, from, to time.Time) []any {
	if !from.IsZero() {
		q.appendAnd("s.start_time >= fromUnixTimestamp64Nano(?)")
		args = append(args, from.UnixNano())
	}
	if !to.IsZero() {
		q.appendAnd("s.start_time <= fromUnixTimestamp64Nano(?)")
		args = append(args, to.UnixNano())
	}
	return args
}

// arrayExistsFn is arrayExists or nestedArrayExists, chosen by the level the attribute lives at.
type arrayExistsFn func(q *sqlBuilder, indent int, prefix string, valueType pcommon.ValueType)

// appendArrayExists writes a condition that checks for a key-value pair in the attribute arrays of
// one level: the span itself when prefix is empty, otherwise the resource or scope arrays.
func (q *sqlBuilder) appendArrayExists(indent int, prefix string, valueType pcommon.ValueType) {
	strColumnType := jptrace.ValueTypeToString(valueType)
	if valueType == pcommon.ValueTypeBytes || valueType == pcommon.ValueTypeMap || valueType == pcommon.ValueTypeSlice {
		strColumnType = "complex"
	}
	columnPrefix := ""
	if prefix != "" {
		columnPrefix = prefix + "_"
	}
	q.appendNewlineAndIndent(indent)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + columnPrefix + strColumnType + "_attributes.key, s." + columnPrefix + strColumnType + "_attributes.value)")
}

// appendNestedArrayExists writes a condition that checks for a key-value pair in nested array
// attributes. Events and links are stored as nested arrays within spans, so a nested
// arrayExists searches through all items and their attributes.
func (q *sqlBuilder) appendNestedArrayExists(indent int, nestedArray string, valueType pcommon.ValueType) {
	strColumnType := jptrace.ValueTypeToString(valueType)
	if valueType == pcommon.ValueTypeBytes || valueType == pcommon.ValueTypeMap || valueType == pcommon.ValueTypeSlice {
		strColumnType = "complex"
	}
	q.appendNewlineAndIndent(indent)
	q.WriteString("arrayExists(x -> arrayExists((key, value) -> key = ? AND value = ?, x." + strColumnType + "_attributes.key, x." + strColumnType + "_attributes.value), s." + nestedArray + ")")
}

// appendStringAttributeFallback writes the condition for an attribute whose type is unknown: a string
// match at every level.
func (q *sqlBuilder) appendStringAttributeFallback(args []any, key string, attr pcommon.Value) []any {
	q.appendArrayExists(2, "", pcommon.ValueTypeStr)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendArrayExists(2, "resource", pcommon.ValueTypeStr)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendArrayExists(2, "scope", pcommon.ValueTypeStr)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendNestedArrayExists(2, "events", pcommon.ValueTypeStr)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendNestedArrayExists(2, "links", pcommon.ValueTypeStr)
	return append(args, key, attr.Str(), key, attr.Str(), key, attr.Str(), key, attr.Str(), key, attr.Str())
}

func buildGetTracesQuery(params tracestore.GetTraceParams) (string, []any) {
	var q sqlBuilder
	q.WriteString(sql.SelectSpansByTraceID)
	args := []any{params.TraceID}
	args = q.appendTimeRange(args, params.Start, params.End)
	return q.String(), args
}

func buildFindTracesQuery(traceIDs []tracestore.FoundTraceID) (string, []any) {
	var q sqlBuilder
	q.WriteString(strings.TrimRight(sql.SelectSpansQuery, "\n"))
	q.WriteString("\nWHERE ")
	args := make([]any, 0, 3*len(traceIDs))
	for i, trace := range traceIDs {
		if i > 0 {
			q.WriteString(" OR ")
		}
		q.WriteString("(s.trace_id = ?")
		args = append(args, trace.TraceID.String())
		// Missing metadata leaves only this trace unbounded.
		if !trace.Start.IsZero() && !trace.End.IsZero() {
			// Positional time.Time arguments are truncated to seconds by clickhouse-go.
			q.WriteString(" AND s.start_time >= fromUnixTimestamp64Nano(?) AND s.start_time <= fromUnixTimestamp64Nano(?)")
			args = append(args, trace.Start.UnixNano(), trace.End.UnixNano())
		}
		q.WriteString(")")
	}
	q.WriteString("\nORDER BY s.trace_id")
	return q.String(), args
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

	// Build the inner subquery that finds distinct trace IDs from spans.
	var inner sqlBuilder
	inner.WriteString(sql.SearchTraceIDsBase)
	args := []any{}

	// The time range bounds either shape, so it is appended after the predicates each shape
	// carries rather than repeated in both branches.
	appendTimeRange := func() {
		if !query.StartTimeMin.IsZero() {
			inner.appendAnd("s.start_time >= ?")
			args = append(args, query.StartTimeMin)
		}
		if !query.StartTimeMax.IsZero() {
			inner.appendAnd("s.start_time <= ?")
			args = append(args, query.StartTimeMax)
		}
	}

	if query.Filter != nil {
		// The query service refuses a query that carries both a filter and the legacy
		// scalar fields (EnsureFilterStandsAlone), so the two shapes are alternatives here
		// rather than combined. A remote-storage client reaches this reader without that
		// check, so it is repeated rather than letting the legacy fields be dropped silently.
		if err := query.EnsureFilterStandsAlone(); err != nil {
			return "", nil, err
		}
		// The untyped attribute metadata for the whole tree is looked up once here, and the
		// lowering reads it from that map.
		metadata, err := r.lookupUntypedMetadata(ctx, query.Filter)
		if err != nil {
			return "", nil, err
		}

		inner.appendAnd("(")
		args, err = inner.buildFilterCondition(2, args, metadata, query.Filter)
		if err != nil {
			return "", nil, err
		}
		inner.appendNewlineAndIndent(1)
		inner.WriteString(")")
		appendTimeRange()
	} else {
		if query.ServiceName != "" {
			inner.appendAnd("s.service_name = ?")
			args = append(args, query.ServiceName)
		}
		if query.OperationName != "" {
			inner.appendAnd("s.name = ?")
			args = append(args, query.OperationName)
		}
		if query.DurationMin > 0 {
			inner.appendAnd("s.duration >= ?")
			args = append(args, query.DurationMin.Nanoseconds())
		}
		if query.DurationMax > 0 {
			inner.appendAnd("s.duration <= ?")
			args = append(args, query.DurationMax.Nanoseconds())
		}
		appendTimeRange()

		attributeMetadata, err := r.getAttributeMetadata(ctx, query.Attributes)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get attribute metadata: %w", err)
		}

		args, err = inner.buildAttributeConditions(args, query.Attributes, attributeMetadata)
		if err != nil {
			return "", nil, err
		}
	}

	inner.WriteString("\nLIMIT ?")
	args = append(args, limit)

	return inner.String(), args, nil
}

func (q *sqlBuilder) buildAttributeConditions(args []any, attributes pcommon.Map, metadata attributeMetadata) ([]any, error) {
	for key, attr := range attributes.All() {
		q.appendAnd("(")

		var err error
		switch attr.Type() {
		case pcommon.ValueTypeBool:
			args = q.buildSimpleAttributeCondition(args, key, pcommon.ValueTypeBool, attr.Bool())
		case pcommon.ValueTypeDouble:
			args = q.buildSimpleAttributeCondition(args, key, pcommon.ValueTypeDouble, attr.Double())
		case pcommon.ValueTypeInt:
			args = q.buildSimpleAttributeCondition(args, key, pcommon.ValueTypeInt, attr.Int())
		case pcommon.ValueTypeStr:
			args = q.buildStringAttributeCondition(args, key, attr, metadata)
		case pcommon.ValueTypeBytes:
			args = q.buildBytesAttributeCondition(args, key, attr)
		case pcommon.ValueTypeSlice:
			args, err = q.buildSliceAttributeCondition(args, key, attr)
			if err != nil {
				return args, err
			}
		case pcommon.ValueTypeMap:
			args, err = q.buildMapAttributeCondition(args, key, attr)
			if err != nil {
				return args, err
			}
		default:
			return args, fmt.Errorf("unsupported attribute type %v for key %s", attr.Type(), key)
		}

		q.appendNewlineAndIndent(1)
		q.WriteString(")")
	}

	return args, nil
}

func (q *sqlBuilder) buildSimpleAttributeCondition(args []any, key string, valueType pcommon.ValueType, value any) []any {
	q.appendArrayExists(2, "", valueType)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendArrayExists(2, "resource", valueType)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendArrayExists(2, "scope", valueType)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendNestedArrayExists(2, "events", valueType)
	q.appendNewlineAndIndent(2)
	q.WriteString("OR")
	q.appendNestedArrayExists(2, "links", valueType)
	return append(args, key, value, key, value, key, value, key, value, key, value)
}

func (q *sqlBuilder) buildBytesAttributeCondition(args []any, key string, attr pcommon.Value) []any {
	return q.buildSimpleAttributeCondition(args, "@bytes@"+key, pcommon.ValueTypeBytes, base64.StdEncoding.EncodeToString(attr.Bytes().AsRaw()))
}

func (q *sqlBuilder) buildSliceAttributeCondition(args []any, key string, attr pcommon.Value) ([]any, error) {
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return args, fmt.Errorf("failed to marshal slice attribute %q: %w", key, err)
	}
	return q.buildSimpleAttributeCondition(args, "@slice@"+key, pcommon.ValueTypeSlice, b), nil
}

func (q *sqlBuilder) buildMapAttributeCondition(args []any, key string, attr pcommon.Value) ([]any, error) {
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return args, fmt.Errorf("failed to marshal map attribute %q: %w", key, err)
	}
	return q.buildSimpleAttributeCondition(args, "@map@"+key, pcommon.ValueTypeMap, b), nil
}

func parseStringToTypedValue(key string, attr pcommon.Value, t pcommon.ValueType) (typedAttributeValue, error) {
	switch t {
	case pcommon.ValueTypeBool:
		b, parseErr := strconv.ParseBool(attr.Str())
		if parseErr != nil {
			return typedAttributeValue{}, fmt.Errorf("failed to parse bool attribute %q: %w", key, parseErr)
		}
		return typedAttributeValue{key: key, value: b, valueType: t}, nil
	case pcommon.ValueTypeDouble:
		f, parseErr := strconv.ParseFloat(attr.Str(), 64)
		if parseErr != nil {
			return typedAttributeValue{}, fmt.Errorf("failed to parse double attribute %q: %w", key, parseErr)
		}
		return typedAttributeValue{key: key, value: f, valueType: t}, nil
	case pcommon.ValueTypeInt:
		i, parseErr := strconv.ParseInt(attr.Str(), 10, 64)
		if parseErr != nil {
			return typedAttributeValue{}, fmt.Errorf("failed to parse int attribute %q: %w", key, parseErr)
		}
		return typedAttributeValue{key: key, value: i, valueType: t}, nil
	case pcommon.ValueTypeStr:
		return typedAttributeValue{key: key, value: attr.Str(), valueType: t}, nil
	case pcommon.ValueTypeBytes:
		return typedAttributeValue{key: "@bytes@" + key, value: attr.Str(), valueType: t}, nil
	case pcommon.ValueTypeMap:
		return typedAttributeValue{key: "@map@" + key, value: attr.Str(), valueType: t}, nil
	case pcommon.ValueTypeSlice:
		return typedAttributeValue{key: "@slice@" + key, value: attr.Str(), valueType: t}, nil
	default:
		return typedAttributeValue{}, fmt.Errorf("unsupported attribute type %v for key %q", t, key)
	}
}

// buildStringAttributeCondition adds a condition for string attributes by looking up their
// actual stored type(s) and level(s) from the attribute_metadata table.
//
// String attributes require special handling because the query service passes all
// attributes as strings (via AsString()), regardless of their actual stored type.
// We must look up the attribute_metadata to determine the actual type(s) and
// level(s) where this attribute is stored, then convert the string back to the
// appropriate type for querying.
//
// If metadata exists but the value cannot be parsed as any of the metadata types,
// we fall back to treating it as a string attribute.
func (q *sqlBuilder) buildStringAttributeCondition(
	args []any,
	key string,
	attr pcommon.Value,
	metadata attributeMetadata,
) []any {
	levelTypes, ok := metadata[key]

	// if no metadata found, assume string type
	if !ok {
		return q.appendStringAttributeFallback(args, key, attr)
	}

	generatedCondition := false
	appendLevel := func(types []pcommon.ValueType, prefix string, fn arrayExistsFn) {
		for _, t := range types {
			tav, err := parseStringToTypedValue(key, attr, t)
			if err != nil {
				// Skip types that can't parse this value
				continue
			}

			if generatedCondition {
				q.appendNewlineAndIndent(2)
				q.WriteString("OR")
			}
			generatedCondition = true

			fn(q, 2, prefix, tav.valueType)
			args = append(args, tav.key, tav.value)
		}
	}

	appendLevel(levelTypes.resource, "resource", (*sqlBuilder).appendArrayExists)
	appendLevel(levelTypes.scope, "scope", (*sqlBuilder).appendArrayExists)
	appendLevel(levelTypes.span, "", (*sqlBuilder).appendArrayExists)
	appendLevel(levelTypes.event, "events", (*sqlBuilder).appendNestedArrayExists)
	appendLevel(levelTypes.link, "links", (*sqlBuilder).appendNestedArrayExists)

	// If no conditions were generated (all types failed to parse),
	// fall back to treating it as a string attribute
	if !generatedCondition {
		return q.appendStringAttributeFallback(args, key, attr)
	}
	return args
}

func buildSelectAttributeMetadataQuery(attributes pcommon.Map) (string, []any) {
	args := []any{}
	var placeholders []string

	for key, attr := range attributes.All() {
		if attr.Type() == pcommon.ValueTypeStr {
			placeholders = append(placeholders, "?")
			args = append(args, key)
		}
	}

	var q sqlBuilder
	q.WriteString(sql.SelectAttributeMetadata)
	if len(placeholders) > 0 {
		q.appendNewlineAndIndent(0)
		q.WriteString("WHERE")
		q.appendNewlineAndIndent(1)
		q.WriteString("attribute_key IN (")
		q.WriteString(strings.Join(placeholders, ", "))
		q.WriteString(")")
	}
	q.appendNewlineAndIndent(0)
	q.WriteString("GROUP BY")
	q.appendNewlineAndIndent(1)
	q.WriteString("attribute_key,")
	q.appendNewlineAndIndent(1)
	q.WriteString("type,")
	q.appendNewlineAndIndent(1)
	q.WriteString("level")
	return q.String(), args
}
