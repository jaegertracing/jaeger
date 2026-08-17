// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// legacyFilterLevels are the attribute levels every backend indexes, and so the levels a
// predicate can be widened to an unqualified tag search over. Instrumentation attributes
// are folded into span tags and link attributes are not stored at all, so a predicate at
// either level cannot be answered by widening (RFC 0005 §1.6).
var legacyFilterLevels = FilterCapabilities{
	Levels: []expression.Level{
		expression.LevelSpan,
		expression.LevelResource,
		expression.LevelEvent,
	},
}

// ToFilterShape expresses q's scalar predicate fields as filter predicates, so that a caller
// which understands only filters — a query interceptor — sees every predicate in one place
// instead of a filter beside four fields that can say the same thing twice.
//
// It cannot fail: each field has an exact filter equivalent, which is why ToLegacyShape is
// able to invert it. A query that already carries a Filter is returned as it is.
//
// The conjunction appears only where there is something to conjoin: no predicates leave a nil
// Filter and a single predicate stands on its own, because `and` is defined to take two
// arguments or more and a tree that wraps fewer is one no validator accepts.
func (q TraceQueryParams) ToFilterShape() TraceQueryParams {
	converted := q
	if converted.Filter != nil {
		return converted
	}
	var predicates []*expression.Call
	compare := func(op expression.Operator, ref expression.Expression, value expression.Expression) {
		predicates = append(predicates, &expression.Call{
			Op:   op,
			Args: []expression.Expression{ref, value},
		})
	}
	field := func(level expression.Level, name string) *expression.FieldRef {
		return &expression.FieldRef{Level: level, Name: name}
	}
	if q.ServiceName != "" {
		compare(expression.OpEq,
			field(expression.LevelResource, expression.ResourceFieldService),
			&expression.StringValue{Value: q.ServiceName})
	}
	if q.OperationName != "" {
		compare(expression.OpEq,
			field(expression.LevelSpan, expression.SpanFieldName),
			&expression.StringValue{Value: q.OperationName})
	}
	// Attributes is documented as needing pcommon.NewMap(), but a caller that left it at its
	// zero value used to reach storage unharmed, and converting shape must not be what turns
	// that into a panic. The zero Map holds no slice to range over.
	if q.Attributes != (pcommon.Map{}) {
		q.Attributes.Range(func(key string, value pcommon.Value) bool {
			// A tag carries no type, so the equality it becomes declares none either and matches
			// the attribute in whatever form it was stored (RFC 0005 §5.4).
			compare(expression.OpEq,
				&expression.AttributeRef{Key: key},
				&expression.AnyValue{Value: value.AsString()})
			return true
		})
	}
	if q.DurationMin != 0 {
		compare(expression.OpGte,
			field(expression.LevelSpan, expression.SpanFieldDuration),
			&expression.DurationValue{Value: q.DurationMin})
	}
	if q.DurationMax != 0 {
		compare(expression.OpLte,
			field(expression.LevelSpan, expression.SpanFieldDuration),
			&expression.DurationValue{Value: q.DurationMax})
	}

	converted.ServiceName = ""
	converted.OperationName = ""
	converted.Attributes = pcommon.NewMap()
	converted.DurationMin = 0
	converted.DurationMax = 0
	switch len(predicates) {
	case 0:
		converted.Filter = nil
	case 1:
		converted.Filter = predicates[0]
	default:
		args := make([]expression.Expression, 0, len(predicates))
		for _, predicate := range predicates {
			args = append(args, predicate)
		}
		converted.Filter = &expression.Call{Op: expression.OpAnd, Args: args}
	}
	return converted
}

// ToLegacyShape expresses q's Filter in the scalar predicate fields every backend serves, so
// a filter query returns the traces the equivalent older query would. It is what a Reader that
// declares no filter support receives.
//
// Only what those fields can carry converts: a flat conjunction of equalities over the
// attribute levels every backend indexes — a backend searching all of them answers with a
// superset — plus the service, the operation name and the inclusive duration bounds.
// Everything else is refused with ErrFilterUnsupported, because widening a level a backend
// never indexed, or reading `gt` as the inclusive bound the field carries, would answer a
// different question than the one asked. ToFilterShape is the inverse and cannot fail.
func (q TraceQueryParams) ToLegacyShape() (TraceQueryParams, error) {
	query := q
	if query.Filter == nil {
		return query, nil
	}
	predicates, err := flatConjuncts(query.Filter)
	if err != nil {
		return query, err
	}
	rewritten := query
	rewritten.Filter = nil
	// A fresh map, so the rewrite cannot be seen through the caller's own query.
	rewritten.Attributes = pcommon.NewMap()
	for _, predicate := range predicates {
		if err := applyAsLegacyField(&rewritten, predicate); err != nil {
			return query, err
		}
	}
	return rewritten, nil
}

// flatConjuncts returns the leaf predicates of a conjunction. A nested `and` means the same
// as a flat one, so it is flattened rather than refused; `or` and `not` have no legacy form,
// so they are refused as the operators they are.
func flatConjuncts(filter *expression.Call) ([]*expression.Call, error) {
	switch filter.Op {
	case expression.OpOr, expression.OpNot:
		return nil, errUnsupportedOperator(filter.Op)
	case expression.OpAnd:
		conjuncts := make([]*expression.Call, 0, len(filter.Args))
		for _, arg := range filter.Args {
			nested, ok := arg.(*expression.Call)
			if !ok {
				return nil, fmt.Errorf("%w: it evaluates a conjunction of predicates only", ErrFilterUnsupported)
			}
			inner, err := flatConjuncts(nested)
			if err != nil {
				return nil, err
			}
			conjuncts = append(conjuncts, inner...)
		}
		return conjuncts, nil
	default:
		return []*expression.Call{filter}, nil
	}
}

// applyAsLegacyField writes one predicate into the legacy field that carries it. A legacy field
// holds a key and one value, so a predicate that reads no single value off the span — a
// quantifier over the events, an operator applied to a call — has nowhere to go.
func applyAsLegacyField(query *TraceQueryParams, predicate *expression.Call) error {
	if len(predicate.Args) != 2 {
		return errUnsupportedOperator(predicate.Op)
	}
	switch ref := predicate.Args[0].(type) {
	case *expression.AttributeRef:
		return applyAttribute(query, predicate.Op, ref, predicate.Args[1])
	case *expression.FieldRef:
		return applyField(query, predicate.Op, ref, predicate.Args[1])
	default:
		return fmt.Errorf("%w: it compares a reference against a constant only", ErrFilterUnsupported)
	}
}

func applyAttribute(query *TraceQueryParams, op expression.Operator, ref *expression.AttributeRef, value expression.Expression) error {
	if op != expression.OpEq {
		return errUnsupportedOperatorOn(op, ref.Key)
	}
	if !legacyFilterLevels.SupportsLevel(ref.Level) {
		return fmt.Errorf("%w: it does not index the %q level", ErrFilterUnsupported, ref.Level)
	}
	text, err := textConstant(ref.Key, value)
	if err != nil {
		return err
	}
	if _, ok := query.Attributes.Get(ref.Key); ok {
		return errRepeatedPredicateOn(ref.Key)
	}
	query.Attributes.PutStr(ref.Key, text)
	return nil
}

// applyField writes a predicate on a built-in field into the legacy field that holds it. Only
// three of the built-ins have one, and a predicate on any of the others is refused.
func applyField(query *TraceQueryParams, op expression.Operator, ref *expression.FieldRef, value expression.Expression) error {
	switch {
	case isField(ref, expression.LevelResource, expression.ResourceFieldService):
		return applyText(&query.ServiceName, op, ref.Name, value)
	case isField(ref, expression.LevelSpan, expression.SpanFieldName):
		return applyText(&query.OperationName, op, ref.Name, value)
	case isField(ref, expression.LevelSpan, expression.SpanFieldDuration):
		return applyDurationBound(query, op, ref.Name, value)
	default:
		return fmt.Errorf("%w: it does not support the built-in field %q of the %q level",
			ErrFilterUnsupported, ref.Name, ref.Level)
	}
}

// isField reports whether the reference names that built-in field. It takes both the level and
// the name because neither identifies a field on its own.
func isField(ref *expression.FieldRef, level expression.Level, name string) bool {
	return ref.Level == level && ref.Name == name
}

// applyText writes an equality on a string-valued legacy field.
func applyText(target *string, op expression.Operator, name string, value expression.Expression) error {
	if op != expression.OpEq {
		return errUnsupportedOperatorOn(op, name)
	}
	text, err := textConstant(name, value)
	if err != nil {
		return err
	}
	if *target != "" {
		return errRepeatedPredicateOn(name)
	}
	*target = text
	return nil
}

// applyDurationBound writes one of the inclusive duration bounds. The constant already holds a
// time.Duration, because expression.ResolveConstants read it as the type span.duration declares
// and refused a spelling that is not one.
func applyDurationBound(query *TraceQueryParams, op expression.Operator, name string, value expression.Expression) error {
	constant, ok := value.(*expression.DurationValue)
	if !ok {
		return errNotADuration(name, value)
	}
	switch op {
	case expression.OpGte:
		if query.DurationMin != 0 {
			return errRepeatedPredicateOn(name)
		}
		query.DurationMin = constant.Value
	case expression.OpLte:
		if query.DurationMax != 0 {
			return errRepeatedPredicateOn(name)
		}
		query.DurationMax = constant.Value
	default:
		return errUnsupportedOperatorOn(op, name)
	}
	return nil
}

// textConstant reads the constant a string-valued legacy field can carry: a text constant, or an
// untyped one, which is what an unqualified tag equality has always been. A constant of any other
// type asks for a match on a type these fields cannot name.
func textConstant(name string, value expression.Expression) (string, error) {
	switch constant := value.(type) {
	case *expression.StringValue:
		return constant.Value, nil
	case *expression.AnyValue:
		return constant.Value, nil
	default:
		return "", fmt.Errorf("%w: it compares %q against a string constant only", ErrFilterUnsupported, name)
	}
}

// errNotADuration refuses a bound that is not a length of time. An untyped constant reaching here
// was never read as a duration, which expression.ResolveConstants does on the way in, so the
// refusal says that rather than blaming the spelling the caller wrote.
func errNotADuration(name string, value expression.Expression) error {
	if constant, ok := value.(*expression.AnyValue); ok {
		return fmt.Errorf("%w: the bound %q on %q was never read as a duration",
			ErrFilterInvalid, constant.Value, name)
	}
	return fmt.Errorf(`%w: it compares %q against a duration such as "2s" only`,
		ErrFilterUnsupported, name)
}

func errUnsupportedOperator(op expression.Operator) error {
	return fmt.Errorf("%w: it does not support the operator %q", ErrFilterUnsupported, op)
}

func errUnsupportedOperatorOn(op expression.Operator, name string) error {
	return fmt.Errorf("%w: it does not support the operator %q on %q", ErrFilterUnsupported, op, name)
}

func errRepeatedPredicateOn(name string) error {
	return fmt.Errorf("%w: it can carry only one predicate on %q", ErrFilterUnsupported, name)
}
