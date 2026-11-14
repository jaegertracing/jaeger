// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// AttributeLayout is the way the spans table stores a typed attribute group, such as
// str_attributes. Everything that depends on that choice is a method of the layout, so no
// statement builder asks which layout it works with. NestedLayout and MapLayout implement it.
type AttributeLayout interface {
	// insertSpan returns the statement that inserts a span.
	insertSpan() string
	// appendAttributes appends the values that the INSERT statement binds for the five typed
	// attribute groups of a span, a resource or a scope.
	appendAttributes(values []any, attributes dbmodel.Attributes) []any
	// appendAttributes2D appends the values that the INSERT statement binds for the five
	// typed groups of the events or of the links. Such a group is one column in every layout,
	// and its value holds the attributes of every event or link of the span.
	appendAttributes2D(values []any, attributes dbmodel.Attributes2D) []any
	// selectSpans returns the statement that reads spans, up to and including its FROM
	// clause. Every layout selects the same columns in the same order.
	selectSpans() string
	// appendComparison writes to q the condition that an attribute of the given level has
	// the key of tav and a value that compares to the value of tav as op says. It returns
	// args extended with the values that the condition binds.
	appendComparison(q *sqlBuilder, args []any, level expression.Level, op expression.Operator, tav typedAttributeValue) []any
}
