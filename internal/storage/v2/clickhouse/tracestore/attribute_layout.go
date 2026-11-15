// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"

// AttributeLayout is the way the spans table stores a typed attribute group, such as
// str_attributes. Everything that depends on that choice is a method of the layout, so no
// statement builder asks which layout it works with. NestedLayout and MapLayout implement it.
//
// A method whose name ends in SQL returns SQL text that binds no values. A bind method
// returns the values that the insert statement binds.
type AttributeLayout interface {
	// insertSpanSQL returns the statement that inserts a span.
	insertSpanSQL() string

	// bindAttributes appends the values that the insert statement binds for the five typed
	// attribute groups of a span, a resource or a scope.
	bindAttributes(values []any, attributes dbmodel.Attributes) []any
	// bindAttributes2D appends the values that the insert statement binds for the five typed
	// groups of the events or of the links. Such a group is one column in every layout, and
	// its value holds the attributes of every event or link of the span.
	bindAttributes2D(values []any, attributes dbmodel.Attributes2D) []any
}
