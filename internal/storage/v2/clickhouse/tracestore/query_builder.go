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

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

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

type typedAttributeValue struct {
	attributeKey string
	value        any
	columnType   string
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

func appendArrayExists(q *strings.Builder, indent int, prefix, colType string) {
	appendNewlineAndIndent(q, indent)
	q.WriteString("arrayExists((key, value) -> key = ? AND value = ?, s." + prefix + colType + "_attributes.key, s." + prefix + colType + "_attributes.value)")
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
	appendArrayExists(q, 2, "", "bool")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", "bool")
	*args = append(*args, key, attr.Bool(), key, attr.Bool())
}

func buildDoubleAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendArrayExists(q, 2, "", "double")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", "double")
	*args = append(*args, key, attr.Double(), key, attr.Double())
}

func buildIntAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	appendArrayExists(q, 2, "", "int")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", "int")
	*args = append(*args, key, attr.Int(), key, attr.Int())
}

func buildBytesAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) {
	attrKey := "@bytes@" + key
	val := base64.StdEncoding.EncodeToString(attr.Bytes().AsRaw())
	appendArrayExists(q, 2, "", "complex")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", "complex")
	*args = append(*args, attrKey, val, attrKey, val)
}

func buildSliceAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@slice@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal slice attribute %q: %w", key, err)
	}
	appendArrayExists(q, 2, "", "complex")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", "complex")
	*args = append(*args, attrKey, b, attrKey, b)
	return nil
}

func buildMapAttributeCondition(q *strings.Builder, args *[]any, key string, attr pcommon.Value) error {
	attrKey := "@map@" + key
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal map attribute %q: %w", key, err)
	}
	appendArrayExists(q, 2, "", "complex")
	appendNewlineAndIndent(q, 2)
	q.WriteString("OR ")
	appendArrayExists(q, 2, "resource_", "complex")
	*args = append(*args, attrKey, b, attrKey, b)
	return nil
}

func parseStringToTypedValue(key string, attr pcommon.Value, t string) (typedAttributeValue, error) {
	switch t {
	case "bool":
		b, parseErr := strconv.ParseBool(attr.Str())
		if parseErr != nil {
			return typedAttributeValue{}, fmt.Errorf("failed to parse bool attribute %q: %w", key, parseErr)
		}
		return typedAttributeValue{attributeKey: key, value: b, columnType: "bool"}, nil
	case "double":
		f, parseErr := strconv.ParseFloat(attr.Str(), 64)
		if parseErr != nil {
			return typedAttributeValue{}, fmt.Errorf("failed to parse double attribute %q: %w", key, parseErr)
		}
		return typedAttributeValue{attributeKey: key, value: f, columnType: "double"}, nil
	case "int":
		i, parseErr := strconv.ParseInt(attr.Str(), 10, 64)
		if parseErr != nil {
			return typedAttributeValue{}, fmt.Errorf("failed to parse int attribute %q: %w", key, parseErr)
		}
		return typedAttributeValue{attributeKey: key, value: i, columnType: "int"}, nil
	case "str":
		return typedAttributeValue{attributeKey: key, value: attr.Str(), columnType: "str"}, nil
	case "bytes":
		return typedAttributeValue{attributeKey: "@bytes@" + key, value: attr.Str(), columnType: "complex"}, nil
	case "map":
		return typedAttributeValue{attributeKey: "@map@" + key, value: attr.Str(), columnType: "complex"}, nil
	case "slice":
		return typedAttributeValue{attributeKey: "@slice@" + key, value: attr.Str(), columnType: "complex"}, nil
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
func buildStringAttributeCondition(
	q *strings.Builder,
	args *[]any,
	key string,
	attr pcommon.Value,
	metadata attributeMetadata,
) error {
	levelTypes, ok := metadata[key]

	// if no metadata found, assume string type
	if !ok {
		appendArrayExists(q, 2, "", "str")
		appendNewlineAndIndent(q, 2)
		q.WriteString("OR ")
		appendArrayExists(q, 2, "resource_", "str")
		appendNewlineAndIndent(q, 2)
		q.WriteString("OR ")
		appendArrayExists(q, 2, "scope_", "str")
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

			tav, err := parseStringToTypedValue(key, attr, t)
			if err != nil {
				return err
			}

			appendArrayExists(q, 2, prefix, tav.columnType)
			*args = append(*args, tav.attributeKey, tav.value)
		}
		return nil
	}

	if err := appendLevel(levelTypes.resource, "resource_"); err != nil {
		return err
	}
	if err := appendLevel(levelTypes.scope, "scope_"); err != nil {
		return err
	}
	return appendLevel(levelTypes.span, "")
}
