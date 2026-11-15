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

// AttributeGroup is the name of a column of the spans table that stores the attributes of
// one value type at one level. A group of the events or of the links is a column of the
// events or links array.
type AttributeGroup string

const (
	GroupBoolAttributes    AttributeGroup = "bool_attributes"
	GroupDoubleAttributes  AttributeGroup = "double_attributes"
	GroupIntAttributes     AttributeGroup = "int_attributes"
	GroupStrAttributes     AttributeGroup = "str_attributes"
	GroupComplexAttributes AttributeGroup = "complex_attributes"

	GroupResourceBoolAttributes    AttributeGroup = "resource_bool_attributes"
	GroupResourceDoubleAttributes  AttributeGroup = "resource_double_attributes"
	GroupResourceIntAttributes     AttributeGroup = "resource_int_attributes"
	GroupResourceStrAttributes     AttributeGroup = "resource_str_attributes"
	GroupResourceComplexAttributes AttributeGroup = "resource_complex_attributes"

	GroupScopeBoolAttributes    AttributeGroup = "scope_bool_attributes"
	GroupScopeDoubleAttributes  AttributeGroup = "scope_double_attributes"
	GroupScopeIntAttributes     AttributeGroup = "scope_int_attributes"
	GroupScopeStrAttributes     AttributeGroup = "scope_str_attributes"
	GroupScopeComplexAttributes AttributeGroup = "scope_complex_attributes"

	GroupEventsBoolAttributes    AttributeGroup = "events.bool_attributes"
	GroupEventsDoubleAttributes  AttributeGroup = "events.double_attributes"
	GroupEventsIntAttributes     AttributeGroup = "events.int_attributes"
	GroupEventsStrAttributes     AttributeGroup = "events.str_attributes"
	GroupEventsComplexAttributes AttributeGroup = "events.complex_attributes"

	GroupLinksBoolAttributes    AttributeGroup = "links.bool_attributes"
	GroupLinksDoubleAttributes  AttributeGroup = "links.double_attributes"
	GroupLinksIntAttributes     AttributeGroup = "links.int_attributes"
	GroupLinksStrAttributes     AttributeGroup = "links.str_attributes"
	GroupLinksComplexAttributes AttributeGroup = "links.complex_attributes"
)

// AttributeValueType is the ClickHouse type of the values that an attribute group stores.
// The str and the complex groups both store String.
type AttributeValueType string

const (
	ValueTypeBool    AttributeValueType = "Bool"
	ValueTypeFloat64 AttributeValueType = "Float64"
	ValueTypeInt64   AttributeValueType = "Int64"
	ValueTypeString  AttributeValueType = "String"
)

// attributeGroups lists every attribute group.
var attributeGroups = []AttributeGroup{
	GroupBoolAttributes,
	GroupDoubleAttributes,
	GroupIntAttributes,
	GroupStrAttributes,
	GroupComplexAttributes,
	GroupResourceBoolAttributes,
	GroupResourceDoubleAttributes,
	GroupResourceIntAttributes,
	GroupResourceStrAttributes,
	GroupResourceComplexAttributes,
	GroupScopeBoolAttributes,
	GroupScopeDoubleAttributes,
	GroupScopeIntAttributes,
	GroupScopeStrAttributes,
	GroupScopeComplexAttributes,
	GroupEventsBoolAttributes,
	GroupEventsDoubleAttributes,
	GroupEventsIntAttributes,
	GroupEventsStrAttributes,
	GroupEventsComplexAttributes,
	GroupLinksBoolAttributes,
	GroupLinksDoubleAttributes,
	GroupLinksIntAttributes,
	GroupLinksStrAttributes,
	GroupLinksComplexAttributes,
}

// Valid reports whether g is one of the attribute groups of the spans table.
func (g AttributeGroup) Valid() bool {
	return slices.Contains(attributeGroups, g)
}

// Valid reports whether t is one of the value types of an attribute group.
func (t AttributeValueType) Valid() bool {
	switch t {
	case ValueTypeBool, ValueTypeFloat64, ValueTypeInt64, ValueTypeString:
		return true
	default:
		return false
	}
}

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
	// ColumnTypeSQL returns the type of the column that stores an attribute group with values
	// of valueType, as CREATE TABLE declares it: Map(String, Bool) for Bool in the map layout.
	ColumnTypeSQL(valueType AttributeValueType) string
	// Declares reports whether a column of columnType is an attribute group of this layout.
	// columnType is any value of the type column of system.columns, such as Map(String, String),
	// Array(String) or Nested(key String, value String), so every string is a valid argument.
	Declares(columnType string) bool

	// insertSpanSQL returns the statement that inserts a span.
	insertSpanSQL() string
	// selectSpansSQL returns the statement that reads spans, up to and including its FROM
	// clause. Every layout selects the same columns in the same order.
	selectSpansSQL() string

	// KeysSQL returns the expression that yields the keys of a group of a span, a resource or
	// a scope as an array.
	KeysSQL(group AttributeGroup) string
	// Keys2DSQL returns the expression that yields the keys of a group of the events or of
	// the links, as one array of keys per event or link.
	Keys2DSQL(group AttributeGroup) string
	// valueSQL returns the expression that reads the value under key from a group of a span
	// or a resource, and yields NULL when the group has no such key. The key is a quoted
	// string literal. A hoisted column is materialized from this expression.
	// TODO: ORDER BY and SELECT of an attribute can use the same expression.
	valueSQL(group AttributeGroup, key string) string

	// appendComparison writes to q the condition that an attribute of the given level has
	// the key of tav and a value that compares to the value of tav as op says. It returns
	// args extended with the values that the condition binds.
	appendComparison(q *sqlBuilder, args []any, level expression.Level, op expression.Operator, tav typedAttributeValue) []any

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
