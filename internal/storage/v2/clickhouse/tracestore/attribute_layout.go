// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import "strings"

// AttributeLayout is the way the spans table stores a typed attribute group, such as
// str_attributes. The statements that name a group are written by its methods, so the layout
// is decided here and not at each place that builds a statement. The zero value is the
// nested layout.
type AttributeLayout string

const (
	// AttributeLayoutNested stores a group as Nested(key String, value T), which is two
	// parallel arrays.
	AttributeLayoutNested AttributeLayout = "nested"
	// AttributeLayoutMap stores a group as Map(String, T).
	AttributeLayoutMap AttributeLayout = "map"
)

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

// insertSpan returns the statement that inserts a span. A group of the span row takes two
// values in the nested layout, its keys and its values, and one value in the map layout. A
// group of the events or of the links takes one value in both layouts.
func (l AttributeLayout) insertSpan() string {
	var names []string
	for _, column := range spanColumns() {
		if column.kind == rowAttributes && l != AttributeLayoutMap {
			names = append(names, column.name+".key", column.name+".value")
		} else {
			names = append(names, column.name)
		}
	}
	const separator = ",\n        "
	return "\nINSERT INTO\n    spans (\n        " + strings.Join(names, separator) +
		"\n    )\nVALUES\n    (\n        ?" + strings.Repeat(separator+"?", len(names)-1) + "\n    )\n"
}
