// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

// Level is the scope a referenced value lives in. The five explicit levels are the
// OTLP attribute maps; an empty Level means an unqualified attribute, searched at the
// span and resource levels. See RFC 0005 §5.1.
type Level string

const (
	LevelSpan            Level = "span"
	LevelResource        Level = "resource"
	LevelInstrumentation Level = "instrumentation"
	LevelEvent           Level = "event"
	LevelLink            Level = "link"
)

// Operator is what a Call applies to its arguments: a boolean combinator, a
// comparison, a set-membership test, or the existential quantifier over a span's
// events or links. See RFC 0005 §5.3 and §5.5.
type Operator string

const (
	OpAnd    Operator = "and"
	OpOr     Operator = "or"
	OpNot    Operator = "not"
	OpEq     Operator = "eq"
	OpNe     Operator = "ne"
	OpGt     Operator = "gt"
	OpLt     Operator = "lt"
	OpGte    Operator = "gte"
	OpLte    Operator = "lte"
	OpRegex  Operator = "regex"
	OpExists Operator = "exists"
	OpIn     Operator = "in"
	OpNotIn  Operator = "not_in"
	OpSome   Operator = "some"
)

// ValueType is the declared type of a constant. It is optional: empty means the
// backend matches the value at whatever type it was stored, and a type that is set is
// authoritative, so the backend matches only values of that type. See RFC 0005 §5.4.
type ValueType string

const (
	ValueTypeString ValueType = "string"
	ValueTypeInt    ValueType = "int"
	ValueTypeDouble ValueType = "double"
	ValueTypeBool   ValueType = "bool"
)

// Built-in field names, the values a span carries directly rather than in an
// attribute map. A Reference reaches one by naming it at its level with Attr unset:
// FieldService at LevelResource, FieldName and FieldDuration at LevelSpan. See
// RFC 0005 §5.2.
const (
	FieldService  = "service"
	FieldName     = "name"
	FieldDuration = "duration"
)

// Expression is a node in a structured filter: an atom — a Reference to a value on
// the span, or a Scalar or List constant — or a Call applying an operator to argument
// expressions. Only the four types in this package implement it, so a backend can
// switch on the concrete type and cover every case. See RFC 0005 §6.
type Expression interface {
	isExpression()
}

// Reference names a value on the span. At an explicit Level, Attr chooses between the
// built-in field called Name and the entry keyed by Name in that level's attribute
// map. An empty Level is always an attribute, whatever Attr says.
type Reference struct {
	// Name is empty only for the collection itself — an event- or link-level Reference
	// standing for every event or link of the span, which is what OpSome quantifies over.
	Name string
	// Level is empty (unqualified) or one of the five explicit levels.
	Level Level
	// Attr is true for an attribute of Level, false for its built-in field.
	Attr bool
}

// Scalar is a single constant value. The value is carried as a string whatever its
// Type, because a value with a unit — a duration such as "2s" — has no native scalar
// form.
type Scalar struct {
	Value string
	Type  ValueType
}

// List is a homogeneous list constant, the right-hand argument of OpIn and OpNotIn.
// Type applies to every element.
type List struct {
	Values []string
	Type   ValueType
}

// Call applies Op to Args. The arity follows the operator: OpNot and OpExists are
// unary, the comparisons and OpIn/OpNotIn are binary, and OpAnd/OpOr take two or
// more. Because an argument is itself an Expression, a predicate can compare two
// references as readily as a reference against a constant.
type Call struct {
	Op   Operator
	Args []Expression
}

func (*Reference) isExpression() {}
func (*Scalar) isExpression()    {}
func (*List) isExpression()      {}
func (*Call) isExpression()      {}

// FilterCapabilities declares how much of a structured filter a Reader evaluates.
// Every zero value is the least capable answer, so a Reader that returns the zero
// value accepts only a flat conjunction of unqualified predicates.
type FilterCapabilities struct {
	// Levels are the levels a Reference may name. Empty means the Reader can serve only
	// unqualified references.
	Levels []Level
	// Operators are the operators the Reader evaluates. OpAnd, OpOr and OpNot are not
	// listed here: a flat conjunction is always available and Boolean governs the rest.
	Operators []Operator
	// Boolean is true when the Reader evaluates OpOr, OpNot, and nested groups. False
	// means it accepts only a flat conjunction.
	Boolean bool
}

// SupportsLevel reports whether a Reference at the given level may reach the Reader.
// An unqualified reference always may.
func (c FilterCapabilities) SupportsLevel(level Level) bool {
	if level == "" {
		return true
	}
	for _, l := range c.Levels {
		if l == level {
			return true
		}
	}
	return false
}

// SupportsOperator reports whether the Reader evaluates the given operator. OpAnd is
// always supported, and OpOr and OpNot follow Boolean rather than the Operators list.
func (c FilterCapabilities) SupportsOperator(op Operator) bool {
	switch op {
	case OpAnd:
		return true
	case OpOr, OpNot:
		return c.Boolean
	}
	for _, o := range c.Operators {
		if o == op {
			return true
		}
	}
	return false
}
