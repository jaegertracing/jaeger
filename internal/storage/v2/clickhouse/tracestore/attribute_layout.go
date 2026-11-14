// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// AttributeLayout is the way the spans table stores a typed attribute group, such as
// str_attributes. Everything that depends on that choice is a method of the layout, so no
// statement builder asks which layout it works with. NestedLayout and MapLayout implement it.
type AttributeLayout interface {
	// insertColumns returns the column names that the INSERT statement lists for one attribute
	// group of a span, a resource or a scope. For str_attributes the nested layout lists
	// str_attributes.key and str_attributes.value, and the map layout lists str_attributes.
	insertColumns(group string) []string
	// appendAttributes appends the values that the INSERT statement binds to the columns
	// of insertColumns, for the five typed groups of a span, a resource or a scope.
	appendAttributes(values []any, attributes dbmodel.Attributes) []any
	// appendAttributes2D appends the values that the INSERT statement binds for the five
	// typed groups of the events or of the links. Such a group is one column in every layout,
	// and its value holds the attributes of every event or link of the span.
	appendAttributes2D(values []any, attributes dbmodel.Attributes2D) []any
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
			names = append(names, layout.insertColumns(column.name)...)
		} else {
			names = append(names, column.name)
		}
	}
	placeholders := slices.Repeat([]string{"?"}, len(names))
	return fmt.Sprintf(insertSpanFormat, indentedList(names, 2), indentedList(placeholders, 2))
}
