// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"slices"
)

// ErrFilterUnsupported is returned for a well-formed query filter that the storage cannot
// serve — a level it does not index, an operator it has not implemented, or a boolean
// structure a flat index cannot evaluate (RFC 0005 §7). The query is refused rather than
// approximated, so a caller never reads a narrower answer as the whole one. The query
// service returns it for the limits a Reader declared through FilterCapabilities, and a
// Reader returns it for the ones that declaration is too coarse to express — a built-in
// field of a level it serves but does not store, or an operator it serves on some
// references and not others.
var ErrFilterUnsupported = errors.New("this storage backend cannot serve this query filter")

// ErrFilterInvalid is returned for a query filter whose value does not fit the field it
// compares — the kind of mistake a structural check cannot catch, because the filter AST
// deliberately does not carry types (RFC 0005 §6.1).
var ErrFilterInvalid = errors.New("invalid query filter")

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

// Field is a built-in field: a value a span carries directly, rather than one held in an
// attribute map (RFC 0005 §5.2). It pairs the name with the level it belongs to, because
// which fields exist is a property of the level — the name of a span and the name of an
// event are different fields that happen to share a spelling, and neither can be read at
// the other's level.
type Field struct {
	Name  string
	Level Level
}

// The built-in fields this build knows. There is no closed set to enumerate, as there is for
// Level and Operator: a field name shares Reference.Name with arbitrary attribute keys, so a
// name this build does not recognize cannot be told from a key. It passes validation and is
// refused by whichever backend receives it, on the same gate an unsupported operator rides
// (RFC 0005 §5.2). This list therefore grows as backends come to serve more of each level.
var (
	SpanName        = Field{Name: "name", Level: LevelSpan}
	SpanDuration    = Field{Name: "duration", Level: LevelSpan}
	ResourceService = Field{Name: "service", Level: LevelResource}
	EventName       = Field{Name: "name", Level: LevelEvent}
)

// Ref returns the Reference that reads f off a span.
func (f Field) Ref() *Reference {
	return &Reference{Name: f.Name, Level: f.Level}
}

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

// IsAttribute reports whether r names an entry in an attribute map rather than a built-in
// field. It is not the Attr bit on its own: an unqualified reference is an attribute of the
// span or resource however Attr is set, because no built-in field has an unqualified form.
func (r *Reference) IsAttribute() bool {
	return r.Level == "" || r.Attr
}

// IsField reports whether r references the built-in field f. A Reference that IsAttribute
// names an attribute of its level and never that level's built-in field, however it is
// spelled.
func (r *Reference) IsField(f Field) bool {
	return !r.IsAttribute() && r.Level == f.Level && r.Name == f.Name
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

// FilterCapabilities declares how much of a structured filter a Reader evaluates. Nothing
// is implicit, so the zero value serves no filter at all.
type FilterCapabilities struct {
	// Levels are the levels a Reference may name. Empty means the Reader can serve only
	// unqualified references.
	Levels []Level
	// Operators are the operators the Reader evaluates. The boolean combinators are listed
	// here like any other operator: a flat inverted index declares OpAnd and omits OpOr and
	// OpNot, which is what confines it to the conjunctive subset. Nesting is not declared
	// separately, because OpAnd is associative and a caller flattens it before asking.
	Operators []Operator
}

// SupportsLevel reports whether a Reference at the given level may reach the Reader.
// An unqualified reference always may.
func (c FilterCapabilities) SupportsLevel(level Level) bool {
	if level == "" {
		return true
	}
	return slices.Contains(c.Levels, level)
}

// SupportsOperator reports whether the Reader evaluates the given operator.
func (c FilterCapabilities) SupportsOperator(op Operator) bool {
	return slices.Contains(c.Operators, op)
}
