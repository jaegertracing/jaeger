// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

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

func appendArrayExists(q *strings.Builder, indent int, prefix string, valueType pcommon.ValueType) {
	strColumnType := jptrace.ValueTypeToString(valueType)
	if valueType == pcommon.ValueTypeBytes || valueType == pcommon.ValueTypeMap || valueType == pcommon.ValueTypeSlice {
		strColumnType = "complex"
	}
	appendNewlineAndIndent(q, indent)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + prefix + strColumnType + "_attributes.key, s." + prefix + strColumnType + "_attributes.value)")
}

func appendStringAttributeFallback(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendArrayExists(q, 2, "", pcommon.ValueTypeStr)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", pcommon.ValueTypeStr)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "scope_", pcommon.ValueTypeStr)
	*args = append(*args, key, attr.Str(), key, attr.Str(), key, attr.Str())
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

	attributeMetadata, err := r.getAttributeMetadata(ctx, query.Attributes)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get attribute metadata: %w", err)
	}

	if err := buildAttributeConditions(&q, &args, query.Attributes, attributeMetadata); err != nil {
		return "", nil, err
	}

	q.WriteString("\nLIMIT ?")
	args = append(args, limit)

	return q.String(), args, nil
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
			buildStringAttributeCondition(q, args, key, attr, metadata)
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
	appendArrayExists(q, 2, "", pcommon.ValueTypeBool)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", pcommon.ValueTypeBool)
	*args = append(*args, key, attr.Bool(), key, attr.Bool())
}

func buildDoubleAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendArrayExists(q, 2, "", pcommon.ValueTypeDouble)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", pcommon.ValueTypeDouble)
	*args = append(*args, key, attr.Double(), key, attr.Double())
}

func buildIntAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendArrayExists(q, 2, "", pcommon.ValueTypeInt)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", pcommon.ValueTypeInt)
	*args = append(*args, key, attr.Int(), key, attr.Int())
}

func buildBytesAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	attrKey := "@bytes@" + key
	val := base64.StdEncoding.EncodeToString(attr.Bytes().AsRaw())
	appendArrayExists(q, 2, "", pcommon.ValueTypeBytes)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", pcommon.ValueTypeBytes)
	*args = append(*args, attrKey, val, attrKey, val)
}

func buildSliceAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@slice@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal slice attribute %q: %w", key, err)
	}
	appendArrayExists(q, 2, "", pcommon.ValueTypeSlice)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", pcommon.ValueTypeSlice)
	*args = append(*args, attrKey, b, attrKey, b)
	return nil
}

func buildMapAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@map@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal map attribute %q: %w", key, err)
	}
	appendArrayExists(q, 2, "", pcommon.ValueTypeMap)
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", pcommon.ValueTypeMap)
	*args = append(*args, attrKey, b, attrKey, b)
	return nil
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
		return typedAttributeValue{}, fmt.Errorf("unsupported attribute type %q for key %q", t, key)
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
func buildStringAttributeCondition(
	q *strings.Builder,
	args *[]any,
	key string,
	attr pcommon.Value,
	metadata attributeMetadata,
) {
	levelTypes, ok := metadata[key]

	// if no metadata found, assume string type
	if !ok {
		appendStringAttributeFallback(q, args, key, attr)
		return
	}

	generatedCondition := false
	appendLevel := func(types []pcommon.ValueType, prefix string) {
		for _, t := range types {
			tav, err := parseStringToTypedValue(key, attr, t)
			if err != nil {
				// Skip types that can't parse this value
				continue
			}

			if generatedCondition {
				appendNewlineAndIndent(q, 2)
				q.WriteString("OR ")
			}
			generatedCondition = true

			appendArrayExists(q, 2, prefix, tav.valueType)
			*args = append(*args, tav.key, tav.value)
		}
	}

	appendLevel(levelTypes.resource, "resource_")
	appendLevel(levelTypes.scope, "scope_")
	appendLevel(levelTypes.span, "")

	// If no conditions were generated (all types failed to parse),
	// fall back to treating it as a string attribute
	if !generatedCondition {
		appendStringAttributeFallback(q, args, key, attr)
	}
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

	var q strings.Builder
	q.WriteString(sql.SelectAttributeMetadata)
	if len(placeholders) > 0 {
		appendNewlineAndIndent(&q, 0)
		q.WriteString("WHERE")
		appendNewlineAndIndent(&q, 1)
		q.WriteString("attribute_key IN (")
		q.WriteString(strings.Join(placeholders, ", "))
		q.WriteString(")")
	}
	appendNewlineAndIndent(&q, 0)
	q.WriteString("GROUP BY")
	appendNewlineAndIndent(&q, 1)
	q.WriteString("attribute_key,")
	appendNewlineAndIndent(&q, 1)
	q.WriteString("type,")
	appendNewlineAndIndent(&q, 1)
	q.WriteString("level")
	return q.String(), args
}
