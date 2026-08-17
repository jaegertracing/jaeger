// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package expression assembles the query expressions of RFC 0005.
//
// An expression written out as struct literals buries the query in its own scaffolding: a
// two-predicate conjunction is ten lines of nested Call, Args and Reference. Predicate names the
// query instead, so the same conjunction reads as
//
//	var p expression.Predicate
//	p.And(
//		p.Resource().Service.Eq("myservice"),
//		p.Attr("http.route").Matches("/api/.*"),
//	)
//
// Predicate builds the boolean-valued expressions a filter is made of. The same AST is meant to
// carry projection and grouping later (RFC 0005 §4), so those get their own builder type in this
// package rather than crowding this one.
//
// The builder is a convenience over the contract rather than a second contract: every chain
// lowers to the same jaeger-idl AST a caller could have written by hand, and it is the Go sibling
// of the Python SDK's builder. Whoever owns a wire converts the result to the proto message that
// wire carries.
//
// Nothing here type checks the query — comparing a duration against a word builds a valid tree
// that means nothing — because RFC 0005 leaves that to the backend, and ast.ValidateFilter is
// what checks the structure an operator requires.
package expression

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	ast "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// Predicate builds boolean-valued expressions. It holds no state, so a zero value is ready to
// use and two callers never interfere; its methods are the builder's whole surface, which is what
// keeps the vocabulary out of package scope where a caller could reassign it.
type Predicate struct{}

// Span names the values of the span itself.
func (Predicate) Span() SpanLevel {
	at := level(ast.LevelSpan)
	return SpanLevel{
		level:         at,
		TraceID:       at.Field(ast.SpanFieldTraceID),
		SpanID:        at.Field(ast.SpanFieldSpanID),
		ParentSpanID:  at.Field(ast.SpanFieldParentSpanID),
		TraceState:    at.Field(ast.SpanFieldTraceState),
		Name:          at.Field(ast.SpanFieldName),
		Kind:          at.Field(ast.SpanFieldKind),
		StartTime:     at.Field(ast.SpanFieldStartTime),
		EndTime:       at.Field(ast.SpanFieldEndTime),
		Duration:      at.Field(ast.SpanFieldDuration),
		Status:        at.Field(ast.SpanFieldStatus),
		StatusMessage: at.Field(ast.SpanFieldStatusMessage),
	}
}

// Resource names the values of the resource the span came from.
func (Predicate) Resource() ResourceLevel {
	at := level(ast.LevelResource)
	return ResourceLevel{
		level:     at,
		Service:   at.Field(ast.ResourceFieldService),
		SchemaURL: at.Field(ast.ResourceFieldSchemaURL),
	}
}

// Scope names the values of the instrumentation scope that recorded the span.
func (Predicate) Scope() ScopeLevel {
	at := level(ast.LevelScope)
	return ScopeLevel{
		level:     at,
		Name:      at.Field(ast.ScopeFieldName),
		Version:   at.Field(ast.ScopeFieldVersion),
		SchemaURL: at.Field(ast.ScopeFieldSchemaURL),
	}
}

// Event names the values of one of the span's events.
func (Predicate) Event() EventLevel {
	at := level(ast.LevelEvent)
	return EventLevel{
		collectionLevel: collectionLevel{at},
		Name:            at.Field(ast.EventFieldName),
		Time:            at.Field(ast.EventFieldTime),
		TimeSinceStart:  at.Field(ast.EventFieldTimeSinceStart),
	}
}

// Link names the values of one of the span's links. The IDs are the linked span's, not the
// linking one's.
func (Predicate) Link() LinkLevel {
	at := level(ast.LevelLink)
	return LinkLevel{
		collectionLevel: collectionLevel{at},
		TraceID:         at.Field(ast.LinkFieldTraceID),
		SpanID:          at.Field(ast.LinkFieldSpanID),
		TraceState:      at.Field(ast.LinkFieldTraceState),
	}
}

// Attr names an attribute without saying which level it lives in, which a backend looks for at
// the span and resource levels. It is the filter counterpart of the legacy attributes map.
func (Predicate) Attr(name string) Ref {
	return Ref{&ast.Reference{Name: name}}
}

// And joins predicates conjunctively. An argument that is itself an and contributes its own
// arguments rather than another level of nesting, and a lone predicate comes back unchanged, so
// the tree stays as flat as a backend restricted to a flat conjunction can read. No predicates
// at all is no filter, which is nil.
func (Predicate) And(predicates ...*ast.Call) *ast.Call {
	return combine(ast.OpAnd, predicates)
}

// Or joins predicates disjunctively, flattening as And does.
func (Predicate) Or(predicates ...*ast.Call) *ast.Call {
	return combine(ast.OpOr, predicates)
}

// Not negates a predicate.
func (Predicate) Not(predicate *ast.Call) *ast.Call {
	return &ast.Call{Op: ast.OpNot, Args: []ast.Expression{predicate}}
}

// Some matches a span holding one event or link that satisfies the predicate. A conjunction
// naming two event fields without the quantifier is uncorrelated, because each conjunct may be
// satisfied by a different event; inside Some both bind to the same one (RFC 0005 §5.5).
func (Predicate) Some(collection Collection, predicate *ast.Call) *ast.Call {
	return &ast.Call{Op: ast.OpSome, Args: []ast.Expression{
		collection.collectionRef(),
		predicate,
	}}
}

// Scalar builds a constant of a declared type, for the comparison that has to narrow the type
// where the operator alone would not (RFC 0005 §5.4).
func (Predicate) Scalar(valueType ast.ValueType, value any) *ast.Scalar {
	return &ast.Scalar{Value: render(value), Type: valueType}
}

// List builds a list constant whose elements are all of a declared type, to pass to In or
// NotIn. A list of values that need no declared type goes to In directly.
func (Predicate) List(valueType ast.ValueType, values ...any) *ast.List {
	return &ast.List{Values: renderAll(values), Type: valueType}
}

// SpanLevel names the span's built-in fields, and the level types below follow the same shape.
type SpanLevel struct {
	level

	TraceID       Ref
	SpanID        Ref
	ParentSpanID  Ref
	TraceState    Ref
	Name          Ref
	Kind          Ref
	StartTime     Ref
	EndTime       Ref
	Duration      Ref
	Status        Ref
	StatusMessage Ref
}

// ResourceLevel names the resource's built-in fields.
type ResourceLevel struct {
	level

	Service   Ref
	SchemaURL Ref
}

// ScopeLevel names the instrumentation scope's built-in fields.
type ScopeLevel struct {
	level

	Name      Ref
	Version   Ref
	SchemaURL Ref
}

// EventLevel names an event's built-in fields.
type EventLevel struct {
	collectionLevel

	Name           Ref
	Time           Ref
	TimeSinceStart Ref
}

// LinkLevel names a link's built-in fields.
type LinkLevel struct {
	collectionLevel

	TraceID    Ref
	SpanID     Ref
	TraceState Ref
}

// level is what every level offers beyond its own named fields.
type level ast.Level

// Attr names an attribute of this level, the case the named fields cannot cover because the key
// is the caller's rather than this API's.
func (l level) Attr(name string) Ref {
	return Ref{&ast.Reference{Name: name, Level: ast.Level(l), Attr: true}}
}

// Field names a built-in field of this level by the spelling ast.Fields lists it under. The named
// fields are what a query written out in Go uses; this is for the caller holding a field name in
// a variable, such as a query arriving from a UI.
func (l level) Field(name string) Ref {
	return Ref{&ast.Reference{Name: name, Level: ast.Level(l)}}
}

// collectionLevel is a level a span holds many of, so a predicate over it can be quantified.
type collectionLevel struct {
	level
}

func (c collectionLevel) collectionRef() *ast.Reference {
	return &ast.Reference{Level: ast.Level(c.level)}
}

// Collection is a level holding many elements per span, which is what Some quantifies over: the
// event and link levels, and no others.
type Collection interface {
	collectionRef() *ast.Reference
}

// Ref is a value named by a query, and the left operand of the predicate its methods build.
type Ref struct {
	ref *ast.Reference
}

func (r Ref) Eq(value any) *ast.Call  { return r.compare(ast.OpEq, value) }
func (r Ref) Ne(value any) *ast.Call  { return r.compare(ast.OpNe, value) }
func (r Ref) Gt(value any) *ast.Call  { return r.compare(ast.OpGt, value) }
func (r Ref) Lt(value any) *ast.Call  { return r.compare(ast.OpLt, value) }
func (r Ref) Gte(value any) *ast.Call { return r.compare(ast.OpGte, value) }
func (r Ref) Lte(value any) *ast.Call { return r.compare(ast.OpLte, value) }

// Matches tests the reference against a regular expression.
func (r Ref) Matches(pattern string) *ast.Call {
	return r.compare(ast.OpRegex, pattern)
}

// Exists tests that the reference has a value at all.
func (r Ref) Exists() *ast.Call {
	return &ast.Call{Op: ast.OpExists, Args: []ast.Expression{r.ref}}
}

// In tests the reference against a list of values.
func (r Ref) In(values ...any) *ast.Call { return r.member(ast.OpIn, values) }

// NotIn is the negation of In.
func (r Ref) NotIn(values ...any) *ast.Call { return r.member(ast.OpNotIn, values) }

// numericOps are the comparisons that ask a backend to read the value as a number, and so the
// only ones that declare the constant's type. Equality and membership leave it open, so they
// match the value in whatever form it was stored (RFC 0005 §5.4).
var numericOps = []ast.Operator{ast.OpGt, ast.OpLt, ast.OpGte, ast.OpLte}

func (r Ref) compare(op ast.Operator, value any) *ast.Call {
	operand := scalarOperand(value, slices.Contains(numericOps, op))
	return &ast.Call{Op: op, Args: []ast.Expression{r.ref, operand}}
}

func (r Ref) member(op ast.Operator, values []any) *ast.Call {
	return &ast.Call{Op: op, Args: []ast.Expression{r.ref, listOperand(values)}}
}

func combine(op ast.Operator, predicates []*ast.Call) *ast.Call {
	var args []ast.Expression
	for _, predicate := range predicates {
		if predicate != nil && predicate.Op == op {
			args = append(args, predicate.Args...)
			continue
		}
		args = append(args, predicate)
	}
	if len(args) == 0 {
		return nil
	}
	if only, ok := args[0].(*ast.Call); ok && len(args) == 1 {
		return only
	}
	return &ast.Call{Op: op, Args: args}
}

// scalarOperand reads the right-hand side of a comparison. Another reference or an already-built
// term is compared as it stands, which is what lets a query compare two references.
func scalarOperand(value any, numeric bool) ast.Expression {
	switch term := value.(type) {
	case Ref:
		return term.ref
	case ast.Expression:
		return term
	}
	scalar := &ast.Scalar{Value: render(value)}
	if numeric {
		scalar.Type = valueTypeOf(value)
	}
	return scalar
}

// listOperand reads the right-hand side of In or NotIn. A list built by Values is taken as it
// stands, which is how a caller declares the element type; anything else becomes a list whose
// elements have no declared type.
func listOperand(values []any) *ast.List {
	if len(values) == 1 {
		if list, ok := values[0].(*ast.List); ok {
			return list
		}
	}
	return &ast.List{Values: renderAll(values)}
}

func renderAll(values []any) []string {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, render(value))
	}
	return rendered
}

// render writes a Go value as the string a constant carries. A duration and an instant are
// spelled the way the built-in fields holding them are compared — Go duration syntax and RFC
// 3339 — so a caller passes the Go value and not its spelling.
func render(value any) string {
	switch term := value.(type) {
	case string:
		return term
	case bool:
		return strconv.FormatBool(term)
	case time.Duration:
		return term.String()
	case time.Time:
		return term.Format(time.RFC3339Nano)
	case float32:
		return strconv.FormatFloat(float64(term), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(term, 'g', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

// valueTypeOf reads the filter type a Go value is compared as. A value of no such type is left
// undeclared rather than guessed at, which is also what a duration string wants.
func valueTypeOf(value any) ast.ValueType {
	switch value.(type) {
	case bool:
		return ast.ValueTypeBool
	case float32, float64:
		return ast.ValueTypeDouble
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return ast.ValueTypeInt
	default:
		return ""
	}
}
