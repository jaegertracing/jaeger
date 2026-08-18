// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package expression assembles the query expressions of RFC 0005.
//
// An expression written out as struct literals buries the query in its own scaffolding: a
// two-predicate conjunction is ten lines of nested Call, Args and reference terms. Predicate
// names the query instead, so the same conjunction reads as
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
// that means nothing — because ast.ValidateFilter is what checks the structure an operator
// requires and ast.ResolveConstants is what reads a constant as the field beside it holds it.
package expression

import (
	"fmt"
	"reflect"
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
		level:          at,
		Name:           at.Field(ast.EventFieldName),
		Time:           at.Field(ast.EventFieldTime),
		TimeSinceStart: at.Field(ast.EventFieldTimeSinceStart),
	}
}

// Link names the values of one of the span's links. The IDs are the linked span's, not the
// linking one's.
func (Predicate) Link() LinkLevel {
	at := level(ast.LevelLink)
	return LinkLevel{
		level:      at,
		TraceID:    at.Field(ast.LinkFieldTraceID),
		SpanID:     at.Field(ast.LinkFieldSpanID),
		TraceState: at.Field(ast.LinkFieldTraceState),
	}
}

// Attr names an attribute without saying which level it lives in, which a backend looks for at
// the span and resource levels. It is the filter counterpart of the legacy attributes map.
func (Predicate) Attr(name string) Ref {
	return Ref{&ast.AttributeRef{Key: name}}
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

// Not builds the negation of a predicate.
func (Predicate) Not(predicate *ast.Call) *ast.Call {
	return &ast.Call{Op: ast.OpNot, Args: []ast.Expression{predicate}}
}

// Some builds a quantifier over the span's events or links, matching a span that holds one
// element satisfying the predicate. A conjunction naming two event fields without the quantifier
// is uncorrelated, because each conjunct may be satisfied by a different event; inside Some both
// bind to the same one (RFC 0005 §5.5).
func (Predicate) Some(collection Collection, predicate *ast.Call) *ast.Call {
	return &ast.Call{Op: ast.OpSome, Args: []ast.Expression{
		collection.nested(),
		predicate,
	}}
}

// Collection is a level a span holds many of, which is what Some may quantify over: the event
// and link levels, and no others. The method is unexported so that no other level can satisfy
// it, which makes quantifying over the span a compile error rather than a refusal at validation.
type Collection interface {
	nested() *ast.NestedRef
}

// Compare builds a comparison in prefix form, for the caller holding an operator in a variable —
// a query arriving from a UI, say. A query written out in Go names the operator instead, through
// the reference's own methods: Duration.Gte(2 * time.Second). Exists is not here because it takes
// no right-hand operand.
func (Predicate) Compare(op ast.Operator, ref Ref, value any) *ast.Call {
	if op == ast.OpIn || op == ast.OpNotIn {
		return ref.member(op, []any{value})
	}
	return ref.compare(op, value)
}

// Text builds a constant to be matched as text, for the comparison that has to narrow the match
// to the string-typed value where a Go string leaves the type open (RFC 0005 §5.4).
func (Predicate) Text(value string) *ast.StringValue {
	return &ast.StringValue{Value: value}
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
	level

	Name           Ref
	Time           Ref
	TimeSinceStart Ref
}

// LinkLevel names a link's built-in fields.
type LinkLevel struct {
	level

	TraceID    Ref
	SpanID     Ref
	TraceState Ref
}

// level is what every level offers beyond its own named fields.
type level ast.Level

// Attr names an attribute of this level, the case the named fields cannot cover because the key
// is the caller's rather than this API's.
func (l level) Attr(name string) Ref {
	return Ref{&ast.AttributeRef{Key: name, Level: ast.Level(l)}}
}

// Field names a built-in field of this level by the spelling ast.Fields lists it under. The named
// fields are what a query written out in Go uses; this is for the caller holding a field name in
// a variable, such as a query arriving from a UI.
func (l level) nested() *ast.NestedRef {
	return &ast.NestedRef{Level: ast.Level(l)}
}

func (l level) Field(name string) Ref {
	return Ref{&ast.FieldRef{Name: name, Level: ast.Level(l)}}
}

// Ref is a value named by a query, and the left operand of the predicate its methods build.
//
// The right-hand operand is any rather than a type parameter because Go does not allow type
// parameters on methods, and a generic function would give up the chained form these methods
// exist for. It is a real union in any case: a Go scalar, another Ref to compare two references,
// or a term already built by Text or List. constant decides which node a Go value becomes, and
// what a backend does with a value it cannot read is RFC 0005's question, not this package's —
// nothing here type checks the query.
type Ref struct {
	ref ast.Expression
}

func (r Ref) Eq(value any) *ast.Call  { return r.compare(ast.OpEq, value) }
func (r Ref) Ne(value any) *ast.Call  { return r.compare(ast.OpNe, value) }
func (r Ref) Gt(value any) *ast.Call  { return r.compare(ast.OpGt, value) }
func (r Ref) Lt(value any) *ast.Call  { return r.compare(ast.OpLt, value) }
func (r Ref) Gte(value any) *ast.Call { return r.compare(ast.OpGte, value) }
func (r Ref) Lte(value any) *ast.Call { return r.compare(ast.OpLte, value) }

// Matches builds a regular-expression test on the reference.
func (r Ref) Matches(pattern string) *ast.Call {
	return r.compare(ast.OpRegex, pattern)
}

// Exists builds a test that the reference has a value at all.
func (r Ref) Exists() *ast.Call {
	return &ast.Call{Op: ast.OpExists, Args: []ast.Expression{r.ref}}
}

// In builds a membership test against a list of values.
func (r Ref) In(values ...any) *ast.Call { return r.member(ast.OpIn, values) }

// NotIn builds the negation of the In test.
func (r Ref) NotIn(values ...any) *ast.Call { return r.member(ast.OpNotIn, values) }

func (r Ref) compare(op ast.Operator, value any) *ast.Call {
	return &ast.Call{Op: op, Args: []ast.Expression{r.ref, operand(value)}}
}

func (r Ref) member(op ast.Operator, values []any) *ast.Call {
	return &ast.Call{Op: op, Args: []ast.Expression{r.ref, r.list(values)}}
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

// operand reads the right-hand side of a comparison. Another reference or an already-built term is
// compared as it stands, which is what lets a query compare two references and what Text uses to
// narrow a match to text.
//
// Everything else becomes an untyped constant, whatever its Go type. A declared type is
// authoritative — `int` matches only what storage kept as an integer — and a Go literal says
// nothing about that: `Attr("size").Gt(500)` asks about the number 500, not about integer storage,
// and typing it would quietly drop a size recorded as 500.0 (RFC 0005 §5.4).
func operand(value any) ast.Expression {
	switch term := value.(type) {
	case Ref:
		return term.ref
	case ast.Expression:
		return term
	}
	return &ast.AnyValue{Value: render(value)}
}

// list reads the right-hand side of In or NotIn. A list built by List is taken as it stands, which
// is how a caller declares the element type outright.
//
// Otherwise the type comes from what the list is compared against: a built-in field declares one
// already, so the list needs none, while an attribute declares nothing and every list has to have
// one (RFC 0005 §5.4). There the type follows the Go values, which is the only statement of intent
// available — and unlike a lone constant, a list cannot decline to make one.
func (r Ref) list(values []any) *ast.List {
	if len(values) == 1 {
		if list, ok := values[0].(*ast.List); ok {
			return list
		}
	}
	elements := renderAll(values)
	if _, ok := r.ref.(*ast.FieldRef); ok {
		return &ast.List{Values: elements}
	}
	return &ast.List{Values: elements, Type: elementType(values)}
}

// elementType names the type a set of Go values was written as. Mixed kinds and anything that is
// not a number or a boolean read as text, which is what a rendered value is.
func elementType(values []any) ast.ValueType {
	kinds := make(map[ast.ValueType]bool, len(values))
	for _, value := range values {
		kinds[valueType(value)] = true
	}
	if len(kinds) == 1 {
		for only := range kinds {
			return only
		}
	}
	return ast.ValueTypeString
}

func valueType(value any) ast.ValueType {
	if _, ok := value.(bool); ok {
		return ast.ValueTypeBool
	}
	// Every integer and floating-point width reaches the same two types, so they are read by kind
	// rather than as a dozen cases naming the same answer. A duration and an instant are text: the
	// wire has no type for either, and only a built-in field's own type can rebuild them.
	switch reflect.ValueOf(value).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return ast.ValueTypeInt
	case reflect.Float32, reflect.Float64:
		return ast.ValueTypeDouble
	default:
		return ast.ValueTypeString
	}
}

func renderAll(values []any) []string {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, render(value))
	}
	return rendered
}

// render writes a Go value as the string a list element carries. A duration and an instant are
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
