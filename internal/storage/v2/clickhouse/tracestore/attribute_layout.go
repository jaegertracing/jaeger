// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"slices"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// AttributeLayout is the way the spans table stores a typed attribute group, such as
// str_attributes. Everything that depends on that choice is a method of the layout, so no
// statement builder asks which layout it works with. NestedLayout and MapLayout implement it.
type AttributeLayout interface {
	// insertColumns returns the columns that the insert statement names for a group of the
	// span row.
	insertColumns(group string) []string
	// appendRowAttributes appends the values that the insert statement binds for the attribute
	// groups of the span row.
	appendRowAttributes(values []any, attributes dbmodel.Attributes) []any
	// appendItemAttributes appends the values that the insert statement binds for the
	// attribute groups of the events or of the links. A group there is one column in every
	// layout, and its value holds the attributes of every item.
	appendItemAttributes(values []any, attributes dbmodel.Attributes2D) []any
}

type spanColumnKind int

const (
	// plainColumn is a column that is not an attribute group.
	plainColumn spanColumnKind = iota
	// rowAttributes is an attribute group of the span row: span, resource or scope.
	rowAttributes
	// itemAttributes is an attribute group that every event or every link has.
	itemAttributes
)

type spanColumn struct {
	name string
	kind spanColumnKind
}

// attributeColumnTypes lists the typed attribute groups in the order of the columns.
var attributeColumnTypes = []string{"bool", "double", "int", "str", "complex"}

// spanColumns lists the columns of a span in the order in which the writer binds them.
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
	groups(rowAttributes, "")
	plain("events.name", "events.timestamp")
	groups(itemAttributes, "events.")
	plain("links.trace_id", "links.span_id", "links.trace_state")
	groups(itemAttributes, "links.")
	plain("service_name")
	groups(rowAttributes, "resource_")
	plain("scope_name", "scope_version")
	groups(rowAttributes, "scope_")
	return columns
}

// insertSpan returns the statement that inserts a span into a table of the given layout.
func insertSpan(layout AttributeLayout) string {
	var names []string
	for _, column := range spanColumns() {
		if column.kind == rowAttributes {
			names = append(names, layout.insertColumns(column.name)...)
		} else {
			names = append(names, column.name)
		}
	}
	var q sqlBuilder
	q.WriteString("INSERT INTO spans (")
	q.appendCommaSeparated(1, names)
	q.appendNewlineAndIndent(0)
	q.WriteString(") VALUES (")
	q.appendCommaSeparated(1, slices.Repeat([]string{"?"}, len(names)))
	q.appendNewlineAndIndent(0)
	q.WriteString(")")
	return q.String()
}
