// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"
	"strings"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// AttributeLayout is the way the spans table stores a typed attribute group, such as
// str_attributes. Everything that depends on that choice is a method of the layout, so no
// statement builder asks which layout it works with. NestedLayout and MapLayout implement it.
//
// A method whose name ends in SQL returns SQL text that binds no values. An append method
// writes SQL to a builder and returns the values that the SQL binds. A bind method returns
// the values that the insert statement binds.
//
// A method is exported when the storage factory calls it, to check the layout of a table
// or to render a schema template. The unexported methods serve the Reader and the Writer
// of this package, and they keep the implementations of a layout inside it.
type AttributeLayout interface {
	// Name returns the value of the attribute_layout setting that selects the layout.
	Name() string
	// ColumnTypeSQL returns the type of the column that stores a group of attributeType, as
	// CREATE TABLE declares it: Map(String, Bool) for a bool group in the map layout.
	ColumnTypeSQL(attributeType AttributeType) string
	// Declares reports whether a column of columnType is an attribute group of this layout.
	// columnType is any value of the type column of system.columns, such as Map(String, String),
	// Array(String) or Nested(key String, value String), so every string is a valid argument.
	Declares(columnType string) bool

	// insertSpanSQL returns the statement that inserts a span.
	insertSpanSQL() string
	// selectSpansSQL returns the statement that reads spans, up to and including its FROM
	// clause. Every layout selects the same columns in the same order.
	selectSpansSQL() string

	// KeysSQL returns the expression that yields the keys of a group. It is one array for a
	// group of a span, a resource or a scope, and one array per event or link for a group of
	// the events or of the links.
	KeysSQL(group AttributeGroup) string
	// valueSQL returns the expression that reads the value under key from a group of a span
	// or a resource, and yields NULL when the group has no such key. The key is a quoted
	// string literal. A hoisted column is materialized from this expression.
	// TODO: ORDER BY and SELECT of an attribute can use the same expression.
	valueSQL(group AttributeGroup, key string) string

	// appendComparison writes to q the condition that an attribute of the given level has
	// the key of tav and a value that compares to the value of tav as op says. It returns
	// args extended with the values that the condition binds.
	appendComparison(q *sqlBuilder, args []any, level expression.Level, op expression.Operator, tav typedAttributeValue) []any

	// appendHasKey writes to q the condition that the group holds the key, and returns args
	// extended with the key. For a group of the events or of the links, the condition is on
	// the one event or link that an enclosing lambda binds to x.
	appendHasKey(q *sqlBuilder, args []any, group AttributeGroup, key string) []any

	// bindAttributes appends the values that the insert statement binds for the five typed
	// attribute groups of a span, a resource or a scope.
	bindAttributes(values []any, attributes dbmodel.Attributes) []any
	// bindAttributes2D appends the values that the insert statement binds for the five typed
	// groups of the events or of the links. Such a group is one column in every layout, and
	// its value holds the attributes of every event or link of the span.
	bindAttributes2D(values []any, attributes dbmodel.Attributes2D) []any
}

// attributeLayouts lists every layout.
var attributeLayouts = []AttributeLayout{
	NestedLayout{},
	MapLayout{},
}

// AttributeLayoutByName returns the layout that the given value of the attribute_layout
// setting selects.
func AttributeLayoutByName(name string) (AttributeLayout, error) {
	names := make([]string, 0, len(attributeLayouts))
	for _, layout := range attributeLayouts {
		if layout.Name() == name {
			return layout, nil
		}
		names = append(names, layout.Name())
	}
	return nil, fmt.Errorf("attribute_layout must be %s, not %q", strings.Join(names, " or "), name)
}
