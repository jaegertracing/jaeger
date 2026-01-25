// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/xpdata"

	"github.com/jaegertracing/jaeger/internal/jptrace"
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

// queryBuilder is a helper for building SQL queries with parameterized arguments.
// It encapsulates both the query string and the arguments slice, providing
// chainable methods for common query building operations.
type queryBuilder struct {
	buf  strings.Builder
	args []any
}

func newQueryBuilder() *queryBuilder {
	return &queryBuilder{args: []any{}}
}

// write appends the given string to the query.
func (q *queryBuilder) write(s string) *queryBuilder {
	q.buf.WriteString(s)
	return q
}

// newline appends a newline followed by the specified indentation level.
func (q *queryBuilder) newline(indent int) *queryBuilder {
	q.buf.WriteString("\n")
	for i := 0; i < indent; i++ {
		q.buf.WriteString("\t")
	}
	return q
}

// arg appends an argument to the args slice.
func (q *queryBuilder) arg(v any) *queryBuilder {
	q.args = append(q.args, v)
	return q
}

// and appends "AND <cond>" with proper formatting.
func (q *queryBuilder) and(cond string) *queryBuilder {
	return q.newline(1).write("AND ").write(cond)
}

// andArg appends "AND <cond>" and adds an argument.
func (q *queryBuilder) andArg(cond string, v any) *queryBuilder {
	return q.and(cond).arg(v)
}

// or appends "OR " at the current position.
func (q *queryBuilder) or() *queryBuilder {
	return q.write("OR ")
}

// openParen appends "AND (" for grouping conditions.
func (q *queryBuilder) openParen() *queryBuilder {
	return q.and("(")
}

// closeParen appends ")" with proper formatting.
func (q *queryBuilder) closeParen() *queryBuilder {
	return q.newline(1).write(")")
}

// arrayExists appends an arrayExists condition for attribute matching.
// prefix is "" for span, "resource_" for resource, or "scope_" for scope attributes.
func (q *queryBuilder) arrayExists(indent int, prefix string, valueType pcommon.ValueType) *queryBuilder {
	strColumnType := jptrace.ValueTypeToString(valueType)
	if valueType == pcommon.ValueTypeBytes || valueType == pcommon.ValueTypeMap || valueType == pcommon.ValueTypeSlice {
		strColumnType = "complex"
	}
	q.newline(indent)
	q.write("arrayExists((key, value) -> key = ? AND value = ?, s." + prefix + strColumnType + "_attributes.key, s." + prefix + strColumnType + "_attributes.value)")
	return q
}

// arrayExistsArg appends arrayExists condition and adds key/value arguments.
func (q *queryBuilder) arrayExistsArg(indent int, prefix string, valueType pcommon.ValueType, key string, value any) *queryBuilder {
	return q.arrayExists(indent, prefix, valueType).arg(key).arg(value)
}

// build returns the final query string and arguments.
func (q *queryBuilder) build() (string, []any) {
	return q.buf.String(), q.args
}

// string returns just the query string.
func (q *queryBuilder) string() string {
	return q.buf.String()
}

func indentBlock(s string) string {
	return "\t" + strings.ReplaceAll(s, "\n", "\n\t")
}

func buildFindTracesQuery(traceIDsQuery string) string {
	inner := indentBlock("SELECT trace_id FROM (\n" + indentBlock(strings.TrimSpace(traceIDsQuery)) + "\n)")
	base := strings.TrimRight(sql.SelectSpansQuery, "\n")
	return base + "\nWHERE s.trace_id IN (\n" + inner + "\n)\nORDER BY s.trace_id"
}

// appendAttributeConditions adds WHERE conditions for all attributes.
func (q *queryBuilder) appendAttributeConditions(attributes pcommon.Map, metadata attributeMetadata) error {
	for key, attr := range attributes.All() {
		q.openParen()

		var err error
		switch attr.Type() {
		case pcommon.ValueTypeBool:
			q.appendSimpleAttributeCondition(key, pcommon.ValueTypeBool, attr.Bool())
		case pcommon.ValueTypeDouble:
			q.appendSimpleAttributeCondition(key, pcommon.ValueTypeDouble, attr.Double())
		case pcommon.ValueTypeInt:
			q.appendSimpleAttributeCondition(key, pcommon.ValueTypeInt, attr.Int())
		case pcommon.ValueTypeStr:
			q.appendStringAttributeCondition(key, attr, metadata)
		case pcommon.ValueTypeBytes:
			q.appendBytesAttributeCondition(key, attr)
		case pcommon.ValueTypeSlice:
			err = q.appendSliceAttributeCondition(key, attr)
		case pcommon.ValueTypeMap:
			err = q.appendMapAttributeCondition(key, attr)
		default:
			err = fmt.Errorf("unsupported attribute type %v for key %s", attr.Type(), key)
		}

		if err != nil {
			return err
		}

		q.closeParen()
	}

	return nil
}

// appendSimpleAttributeCondition appends conditions for simple typed attributes (bool, double, int).
func (q *queryBuilder) appendSimpleAttributeCondition(key string, valueType pcommon.ValueType, value any) {
	q.arrayExistsArg(2, "", valueType, key, value)
	q.newline(2).or()
	q.arrayExistsArg(2, "resource_", valueType, key, value)
}

// appendBytesAttributeCondition appends conditions for bytes attributes.
func (q *queryBuilder) appendBytesAttributeCondition(key string, attr pcommon.Value) {
	encodedValue := base64.StdEncoding.EncodeToString(attr.Bytes().AsRaw())
	q.appendSimpleAttributeCondition("@bytes@"+key, pcommon.ValueTypeBytes, encodedValue)
}

// appendSliceAttributeCondition appends conditions for slice attributes.
func (q *queryBuilder) appendSliceAttributeCondition(key string, attr pcommon.Value) error {
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal slice attribute %q: %w", key, err)
	}
	q.appendSimpleAttributeCondition("@slice@"+key, pcommon.ValueTypeSlice, b)
	return nil
}

// appendMapAttributeCondition appends conditions for map attributes.
func (q *queryBuilder) appendMapAttributeCondition(key string, attr pcommon.Value) error {
	b, err := marshalValueForQuery(attr)
	if err != nil {
		return fmt.Errorf("failed to marshal map attribute %q: %w", key, err)
	}
	q.appendSimpleAttributeCondition("@map@"+key, pcommon.ValueTypeMap, b)
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
		return typedAttributeValue{}, fmt.Errorf("unsupported attribute type %v for key %q", t, key)
	}
}

// appendStringAttributeCondition adds a condition for string attributes by looking up their
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
func (q *queryBuilder) appendStringAttributeCondition(
	key string,
	attr pcommon.Value,
	metadata attributeMetadata,
) {
	levelTypes, ok := metadata[key]

	// if no metadata found, assume string type
	if !ok {
		q.appendStringAttributeFallback(key, attr)
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
				q.newline(2).or()
			}
			generatedCondition = true

			q.arrayExistsArg(2, prefix, tav.valueType, tav.key, tav.value)
		}
	}

	appendLevel(levelTypes.resource, "resource_")
	appendLevel(levelTypes.scope, "scope_")
	appendLevel(levelTypes.span, "")

	// If no conditions were generated (all types failed to parse),
	// fall back to treating it as a string attribute
	if !generatedCondition {
		q.appendStringAttributeFallback(key, attr)
	}
}

// appendStringAttributeFallback appends fallback conditions searching all levels for string attributes.
func (q *queryBuilder) appendStringAttributeFallback(key string, attr pcommon.Value) {
	q.arrayExistsArg(2, "", pcommon.ValueTypeStr, key, attr.Str())
	q.newline(2).or()
	q.arrayExistsArg(2, "resource_", pcommon.ValueTypeStr, key, attr.Str())
	q.newline(2).or()
	q.arrayExistsArg(2, "scope_", pcommon.ValueTypeStr, key, attr.Str())
}

// buildSelectAttributeMetadataQuery builds the query for fetching attribute metadata.
func buildSelectAttributeMetadataQuery(attributes pcommon.Map) (string, []any) {
	q := newQueryBuilder()
	q.write(sql.SelectAttributeMetadata)

	var placeholders []string
	for key, attr := range attributes.All() {
		if attr.Type() == pcommon.ValueTypeStr {
			placeholders = append(placeholders, "?")
			q.arg(key)
		}
	}

	if len(placeholders) > 0 {
		q.newline(0).write("WHERE")
		q.newline(1).write("attribute_key IN (").write(strings.Join(placeholders, ", ")).write(")")
	}

	q.newline(0).write("GROUP BY")
	q.newline(1).write("attribute_key,")
	q.newline(1).write("type,")
	q.newline(1).write("level")

	return q.build()
}
