// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package filter assembles the structured trace-query filter of RFC 0005 §6.3.
//
// The filter is an expression tree, and written out as struct literals that tree buries the
// query in its own scaffolding: a two-predicate conjunction is ten lines of nested Call, Args
// and Reference. The functions here name the query instead, so the same conjunction reads as
//
//	filter.And(
//		filter.Resource.Service.Eq("myservice"),
//		filter.Attr("http.route").Matches("/api/.*"),
//	)
//
// It is a convenience over the contract rather than a second contract: every chain lowers to
// the same jaeger-idl AST a caller could have written by hand, and it is the Go sibling of the
// Python SDK's builder. Whoever owns a wire converts the result to the proto message that wire
// carries.
//
// Nothing here type checks the query — comparing a duration against a word builds a valid tree
// that means nothing — because RFC 0005 leaves that to the backend, and expression.ValidateFilter
// is what checks the structure an operator requires.
package filter

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

var (
	atSpan            = level(expression.LevelSpan)
	atResource        = level(expression.LevelResource)
	atInstrumentation = level(expression.LevelInstrumentation)
	atEvent           = level(expression.LevelEvent)
	atLink            = level(expression.LevelLink)
)

// The five attribute levels, each naming its own built-in fields as members and any attribute
// of that level through Attr. A caller writes filter.Span.Duration or filter.Span.Attr("k"),
// which is why the levels are values rather than a type to convert an expression.Level into.
var (
	Span = SpanLevel{
		level:         atSpan,
		TraceID:       atSpan.Field(expression.SpanFieldTraceID),
		SpanID:        atSpan.Field(expression.SpanFieldSpanID),
		ParentSpanID:  atSpan.Field(expression.SpanFieldParentSpanID),
		TraceState:    atSpan.Field(expression.SpanFieldTraceState),
		Name:          atSpan.Field(expression.SpanFieldName),
		Kind:          atSpan.Field(expression.SpanFieldKind),
		StartTime:     atSpan.Field(expression.SpanFieldStartTime),
		EndTime:       atSpan.Field(expression.SpanFieldEndTime),
		Duration:      atSpan.Field(expression.SpanFieldDuration),
		Status:        atSpan.Field(expression.SpanFieldStatus),
		StatusMessage: atSpan.Field(expression.SpanFieldStatusMessage),
	}

	Resource = ResourceLevel{
		level:     atResource,
		Service:   atResource.Field(expression.ResourceFieldService),
		SchemaURL: atResource.Field(expression.ResourceFieldSchemaURL),
	}

	Instrumentation = InstrumentationLevel{
		level:     atInstrumentation,
		Name:      atInstrumentation.Field(expression.InstrumentationFieldName),
		Version:   atInstrumentation.Field(expression.InstrumentationFieldVersion),
		SchemaURL: atInstrumentation.Field(expression.InstrumentationFieldSchemaURL),
	}

	Event = EventLevel{
		collectionLevel: collectionLevel{atEvent},
		Name:            atEvent.Field(expression.EventFieldName),
		Time:            atEvent.Field(expression.EventFieldTime),
		TimeSinceStart:  atEvent.Field(expression.EventFieldTimeSinceStart),
	}

	Link = LinkLevel{
		collectionLevel: collectionLevel{atLink},
		TraceID:         atLink.Field(expression.LinkFieldTraceID),
		SpanID:          atLink.Field(expression.LinkFieldSpanID),
		TraceState:      atLink.Field(expression.LinkFieldTraceState),
	}
)

// SpanLevel names the values of the span itself. Its fields are the span's built-in fields, and
// the level types below follow the same shape.
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

// ResourceLevel names the values of the resource the span came from.
type ResourceLevel struct {
	level

	Service   Ref
	SchemaURL Ref
}

// InstrumentationLevel names the values of the instrumentation scope that recorded the span.
type InstrumentationLevel struct {
	level

	Name      Ref
	Version   Ref
	SchemaURL Ref
}

// EventLevel names the values of one of the span's events.
type EventLevel struct {
	collectionLevel

	Name           Ref
	Time           Ref
	TimeSinceStart Ref
}

// LinkLevel names the values of one of the span's links. The IDs are the linked span's, not the
// linking one's.
type LinkLevel struct {
	collectionLevel

	TraceID    Ref
	SpanID     Ref
	TraceState Ref
}

// level is what every level offers beyond its own named fields.
type level expression.Level

// Attr names an attribute of this level, the case the named fields cannot cover because the key
// is the caller's rather than this API's.
func (l level) Attr(name string) Ref {
	return Ref{&expression.Reference{Name: name, Level: expression.Level(l), Attr: true}}
}

// Field names a built-in field of this level by the spelling expression.Fields lists it under.
// The named fields are what a query written out in Go uses; this is for the caller holding a
// field name in a variable, such as a query arriving from a UI.
func (l level) Field(name string) Ref {
	return Ref{&expression.Reference{Name: name, Level: expression.Level(l)}}
}

// collectionLevel is a level a span holds many of, so a predicate over it can be quantified.
type collectionLevel struct {
	level
}

func (c collectionLevel) collectionRef() *expression.Reference {
	return &expression.Reference{Level: expression.Level(c.level)}
}

// Collection is a level holding many elements per span, which is what Some quantifies over: the
// event and link levels, and no others.
type Collection interface {
	collectionRef() *expression.Reference
}

// Attr names an attribute without saying which level it lives in, which a backend looks for at
// the span and resource levels. It is the filter counterpart of the legacy attributes map.
func Attr(name string) Ref {
	return Ref{&expression.Reference{Name: name}}
}

// Ref is a value named by a query, and the left operand of the predicate its methods build.
type Ref struct {
	ref *expression.Reference
}

func (r Ref) Eq(value any) *expression.Call  { return r.compare(expression.OpEq, value) }
func (r Ref) Ne(value any) *expression.Call  { return r.compare(expression.OpNe, value) }
func (r Ref) Gt(value any) *expression.Call  { return r.compare(expression.OpGt, value) }
func (r Ref) Lt(value any) *expression.Call  { return r.compare(expression.OpLt, value) }
func (r Ref) Gte(value any) *expression.Call { return r.compare(expression.OpGte, value) }
func (r Ref) Lte(value any) *expression.Call { return r.compare(expression.OpLte, value) }

// Matches tests the reference against a regular expression.
func (r Ref) Matches(pattern string) *expression.Call {
	return r.compare(expression.OpRegex, pattern)
}

// Exists tests that the reference has a value at all.
func (r Ref) Exists() *expression.Call {
	return &expression.Call{Op: expression.OpExists, Args: []expression.Expression{r.ref}}
}

// In tests the reference against a list of values.
func (r Ref) In(values ...any) *expression.Call { return r.member(expression.OpIn, values) }

// NotIn is the negation of In.
func (r Ref) NotIn(values ...any) *expression.Call { return r.member(expression.OpNotIn, values) }

// numericOps are the comparisons that ask a backend to read the value as a number, and so the
// only ones that declare the constant's type. Equality and membership leave it open, so they
// match the value in whatever form it was stored (RFC 0005 §5.4).
var numericOps = []expression.Operator{expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte}

func (r Ref) compare(op expression.Operator, value any) *expression.Call {
	operand := scalarOperand(value, slices.Contains(numericOps, op))
	return &expression.Call{Op: op, Args: []expression.Expression{r.ref, operand}}
}

func (r Ref) member(op expression.Operator, values []any) *expression.Call {
	return &expression.Call{Op: op, Args: []expression.Expression{r.ref, listOperand(values)}}
}

// And joins predicates conjunctively. An argument that is itself an and contributes its own
// arguments rather than another level of nesting, and a lone predicate comes back unchanged, so
// the tree stays as flat as a backend restricted to a flat conjunction can read. No predicates
// at all is no filter, which is nil.
func And(predicates ...*expression.Call) *expression.Call {
	return combine(expression.OpAnd, predicates)
}

// Or joins predicates disjunctively, flattening as And does.
func Or(predicates ...*expression.Call) *expression.Call {
	return combine(expression.OpOr, predicates)
}

// Not negates a predicate.
func Not(predicate *expression.Call) *expression.Call {
	return &expression.Call{Op: expression.OpNot, Args: []expression.Expression{predicate}}
}

// Some matches a span holding one event or link that satisfies the predicate. A conjunction
// naming two event fields without the quantifier is uncorrelated, because each conjunct may be
// satisfied by a different event; inside Some both bind to the same one (RFC 0005 §5.5).
func Some(collection Collection, predicate *expression.Call) *expression.Call {
	return &expression.Call{Op: expression.OpSome, Args: []expression.Expression{
		collection.collectionRef(),
		predicate,
	}}
}

// Value builds a constant of a declared type, for the comparison that has to narrow the type
// where the operator alone would not (RFC 0005 §5.4).
func Value(valueType expression.ValueType, value any) *expression.Scalar {
	return &expression.Scalar{Value: render(value), Type: valueType}
}

// Values builds a list constant whose elements are all of a declared type, to pass to In or
// NotIn. A list of values that need no declared type goes to In directly.
func Values(valueType expression.ValueType, values ...any) *expression.List {
	return &expression.List{Values: renderAll(values), Type: valueType}
}

func combine(op expression.Operator, predicates []*expression.Call) *expression.Call {
	var args []expression.Expression
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
	if only, ok := args[0].(*expression.Call); ok && len(args) == 1 {
		return only
	}
	return &expression.Call{Op: op, Args: args}
}

// scalarOperand reads the right-hand side of a comparison. Another reference or an already-built
// term is compared as it stands, which is what lets a query compare two references.
func scalarOperand(value any, numeric bool) expression.Expression {
	switch term := value.(type) {
	case Ref:
		return term.ref
	case expression.Expression:
		return term
	}
	scalar := &expression.Scalar{Value: render(value)}
	if numeric {
		scalar.Type = valueTypeOf(value)
	}
	return scalar
}

// listOperand reads the right-hand side of In or NotIn. A list built by Values is taken as it
// stands, which is how a caller declares the element type; anything else becomes a list whose
// elements have no declared type.
func listOperand(values []any) *expression.List {
	if len(values) == 1 {
		if list, ok := values[0].(*expression.List); ok {
			return list
		}
	}
	return &expression.List{Values: renderAll(values)}
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
func valueTypeOf(value any) expression.ValueType {
	switch value.(type) {
	case bool:
		return expression.ValueTypeBool
	case float32, float64:
		return expression.ValueTypeDouble
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return expression.ValueTypeInt
	default:
		return ""
	}
}
