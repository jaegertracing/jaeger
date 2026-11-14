// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"
	"slices"
	"strings"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// AttributeLayout is the way the spans table stores a typed attribute group, such as
// str_attributes. Everything that depends on that choice is a method of the layout, so no
// statement builder asks which layout it works with. NestedLayout and MapLayout implement it.
type AttributeLayout interface {
	// columnNames returns the names of the columns that store one attribute group of a span,
	// a resource or a scope, as the INSERT statement lists them. For str_attributes they are
	// str_attributes.key and str_attributes.value in the nested layout, and str_attributes in
	// the map layout.
	columnNames(group string) []string
	// appendAttributes appends the values that the INSERT statement binds to the columns
	// that columnNames returns, for the five typed groups of a span, a resource or a scope.
	appendAttributes(values []any, attributes dbmodel.Attributes) []any
	// appendAttributes2D appends the values that the INSERT statement binds for the five
	// typed groups of the events or of the links. Such a group is one column in every layout,
	// and its value holds the attributes of every event or link of the span.
	appendAttributes2D(values []any, attributes dbmodel.Attributes2D) []any
	// Keys returns the expression for the keys of a group of the span row, and values the
	// expression for its values, in the same order.
	Keys(group string) string
	values(group string) string
	// Keys2D returns the expression for the keys of a group of the events or of the links,
	// which is one array of keys per item, and values2D the expression for its values.
	Keys2D(group string) string
	values2D(group string) string
	// appendEquals writes to q the condition that the attributes of one level hold the key
	// and value of tav, and returns args extended with the values that the condition binds.
	appendEquals(q *sqlBuilder, args []any, level expression.Level, tav typedAttributeValue) []any
}

type spanColumnKind int

const (
	// plainColumn is a column that is not an attribute group.
	plainColumn spanColumnKind = iota
	// attributesColumn is an attribute group of the span row: span, resource or scope.
	attributesColumn
	// attributes2DColumn is an attribute group that every event or every link has.
	attributes2DColumn
)

type spanColumn struct {
	name string
	kind spanColumnKind
}

// attributeColumnTypes lists the typed attribute groups in the order of the columns.
var attributeColumnTypes = []string{"bool", "double", "int", "str", "complex"}

// spanColumns lists the columns of a span in the order in which the writer binds them and
// the reader scans them.
func spanColumns() []spanColumn {
	var columns []spanColumn
	plain := func(names ...string) {
		for _, name := range names {
			columns = append(columns, spanColumn{name: name, kind: plainColumn})
		}
	}
	groups := func(kind spanColumnKind, prefix string) {
		for _, columnType := range attributeColumnTypes {
			columns = append(columns, spanColumn{name: prefix + columnType + "_attributes", kind: kind})
		}
	}
	plain(
		"id",
		"trace_id",
		"trace_state",
		"parent_span_id",
		"name",
		"kind",
		"start_time",
		"status_code",
		"status_message",
		"duration",
	)
	groups(attributesColumn, "")
	plain("events.name", "events.timestamp")
	groups(attributes2DColumn, "events.")
	plain("links.trace_id", "links.span_id", "links.trace_state")
	groups(attributes2DColumn, "links.")
	plain("service_name")
	groups(attributesColumn, "resource_")
	plain("scope_name", "scope_version")
	groups(attributesColumn, "scope_")
	return columns
}

// insertSpanFormat takes the list of the columns and the list of their placeholders.
const insertSpanFormat = `
INSERT INTO
    spans (
        %s
    )
VALUES
    (
        %s
    )
`

// indentedList joins the items with a comma, each item after the first on a line of its own
// at the given depth of four-space indentation.
func indentedList(items []string, depth int) string {
	return strings.Join(items, ",\n"+strings.Repeat("    ", depth))
}

// insertSpan returns the statement that inserts a span into a table of the given layout.
func insertSpan(layout AttributeLayout) string {
	var names []string
	for _, column := range spanColumns() {
		if column.kind == attributesColumn {
			names = append(names, layout.columnNames(column.name)...)
		} else {
			names = append(names, column.name)
		}
	}
	placeholders := slices.Repeat([]string{"?"}, len(names))
	return fmt.Sprintf(insertSpanFormat, indentedList(names, 2), indentedList(placeholders, 2))
}

// selectSpans returns the statement that reads spans from a table of the given layout, up to
// and including its FROM clause. Every layout yields a group as its keys and its values, so
// a span is scanned the same way from any of them.
func selectSpans(layout AttributeLayout) string {
	var expressions []string
	for _, column := range spanColumns() {
		switch column.kind {
		case attributesColumn:
			expressions = append(expressions, layout.Keys(column.name), layout.values(column.name))
		case attributes2DColumn:
			expressions = append(expressions, layout.Keys2D(column.name), layout.values2D(column.name))
		default: // plainColumn
			expressions = append(expressions, column.name)
		}
	}
	var q sqlBuilder
	q.WriteString("SELECT")
	q.appendCommaSeparated(1, expressions)
	q.appendNewlineAndIndent(0)
	q.WriteString("FROM spans s")
	return q.String()
}
