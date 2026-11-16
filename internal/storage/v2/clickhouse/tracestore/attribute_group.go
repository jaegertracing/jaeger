// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// AttributeType is the kind of value that one attribute group stores. Bytes, maps and slices
// share the complex group, where the key carries a prefix that tells them apart.
type AttributeType string

const (
	AttributeTypeBool    AttributeType = "bool"
	AttributeTypeDouble  AttributeType = "double"
	AttributeTypeInt     AttributeType = "int"
	AttributeTypeStr     AttributeType = "str"
	AttributeTypeComplex AttributeType = "complex"
)

// attributeTypeOf returns the type of the group that stores a value of valueType. No group
// stores an empty value, and no caller passes one.
func attributeTypeOf(valueType pcommon.ValueType) AttributeType {
	switch valueType {
	case pcommon.ValueTypeBool:
		return AttributeTypeBool
	case pcommon.ValueTypeDouble:
		return AttributeTypeDouble
	case pcommon.ValueTypeInt:
		return AttributeTypeInt
	case pcommon.ValueTypeStr:
		return AttributeTypeStr
	case pcommon.ValueTypeBytes, pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		return AttributeTypeComplex
	default:
		return ""
	}
}

// Valid reports whether t is one of the attribute types.
func (t AttributeType) Valid() bool {
	switch t {
	case AttributeTypeBool, AttributeTypeDouble, AttributeTypeInt, AttributeTypeStr, AttributeTypeComplex:
		return true
	default:
		return false
	}
}

// valueTypeSQL returns the ClickHouse type of the values of a group of this type. The str and
// the complex groups both store strings.
func (t AttributeType) valueTypeSQL() string {
	switch t {
	case AttributeTypeBool:
		return "Bool"
	case AttributeTypeDouble:
		return "Float64"
	case AttributeTypeInt:
		return "Int64"
	default:
		return "String"
	}
}

// AttributeGroup identifies the column of the spans table that stores the attributes of one
// type at one level.
type AttributeGroup struct {
	Level expression.Level
	Type  AttributeType
}

// NewAttributeGroup returns the group of the level and the type. It fails when the spans
// table has no attributes at the level or no group of the type.
func NewAttributeGroup(level expression.Level, attributeType AttributeType) (AttributeGroup, error) {
	switch level {
	case expression.LevelSpan,
		expression.LevelResource,
		expression.LevelScope,
		expression.LevelEvent,
		expression.LevelLink:
	default:
		return AttributeGroup{}, fmt.Errorf("%q is not a level of attributes", level)
	}
	if !attributeType.Valid() {
		return AttributeGroup{}, fmt.Errorf("%q is not a type of attribute group", attributeType)
	}
	return AttributeGroup{
		Level: level,
		Type:  attributeType,
	}, nil
}

// attributeGroup returns the group that stores the attributes of valueType at the level.
func attributeGroup(level expression.Level, valueType pcommon.ValueType) AttributeGroup {
	return AttributeGroup{
		Level: level,
		Type:  attributeTypeOf(valueType),
	}
}

// field returns the name of the group within its level, such as str_attributes. It is the
// name of the column for a span, and the name of the field of one event or link.
func (g AttributeGroup) field() string {
	return string(g.Type) + "_attributes"
}

// arrayColumn returns the array column that holds the group, which is events or links, with
// one entry per event or link. It returns an empty string for a group of a span, a resource
// or a scope, which is a column of the table itself.
func (g AttributeGroup) arrayColumn() string {
	switch g.Level {
	case expression.LevelEvent:
		return "events"
	case expression.LevelLink:
		return "links"
	default:
		return ""
	}
}

// operandSQL returns the group as a condition names it: the column of the span row s, or,
// for a group of the events or of the links, the field of the one event or link that a lambda
// binds to x.
func (g AttributeGroup) operandSQL() string {
	if g.arrayColumn() != "" {
		return "x." + g.field()
	}
	return "s." + g.column()
}

// column returns the name of the group in the spans table: str_attributes for a span,
// resource_str_attributes, scope_str_attributes, events.str_attributes or links.str_attributes.
func (g AttributeGroup) column() string {
	switch g.Level {
	case expression.LevelResource:
		return "resource_" + g.field()
	case expression.LevelScope:
		return "scope_" + g.field()
	case expression.LevelEvent, expression.LevelLink:
		return g.arrayColumn() + "." + g.field()
	default:
		return g.field()
	}
}
