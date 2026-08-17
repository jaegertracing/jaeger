// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"
	"time"

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
	eq := func(ref *expression.Reference, value string) {
		predicates = append(predicates, &expression.Call{
			Op:   expression.OpEq,
			Args: []expression.Expression{ref, &expression.Scalar{Value: value}},
		})
	}
	if q.ServiceName != "" {
		eq(&expression.Reference{Level: expression.LevelResource, Name: expression.ResourceFieldService}, q.ServiceName)
	}
	if q.OperationName != "" {
		eq(&expression.Reference{Level: expression.LevelSpan, Name: expression.SpanFieldName}, q.OperationName)
	}
	// Attributes is documented as needing pcommon.NewMap(), but a caller that left it at its
	// zero value used to reach storage unharmed, and converting shape must not be what turns
	// that into a panic. The zero Map holds no slice to range over.
	if q.Attributes != (pcommon.Map{}) {
		q.Attributes.Range(func(key string, value pcommon.Value) bool {
			eq(&expression.Reference{Name: key}, value.AsString())
			return true
		})
	}
	if q.DurationMin != 0 {
		predicates = append(predicates, durationBound(expression.OpGte, q.DurationMin))
	}
	if q.DurationMax != 0 {
		predicates = append(predicates, durationBound(expression.OpLte, q.DurationMax))
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

// durationBound writes a duration back in the Go syntax the filter carries it in, which is
// what ToLegacyShape parses to recover it.
func durationBound(op expression.Operator, d time.Duration) *expression.Call {
	return &expression.Call{
		Op: op,
		Args: []expression.Expression{
			&expression.Reference{Level: expression.LevelSpan, Name: expression.SpanFieldDuration},
			&expression.Scalar{Value: d.String()},
		},
	}
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

// applyAsLegacyField writes one predicate into the legacy field that carries it.
func applyAsLegacyField(query *TraceQueryParams, predicate *expression.Call) error {
	ref, value, err := refAndConstant(predicate)
	if err != nil {
		return err
	}
	switch {
	case ref.IsAttribute():
		return applyAttribute(query, predicate.Op, ref, value)
	case ref.IsField(expression.LevelResource, expression.ResourceFieldService):
		if predicate.Op != expression.OpEq {
			return errUnsupportedOperatorOn(predicate.Op, ref)
		}
		if query.ServiceName != "" {
			return errRepeatedPredicateOn(ref)
		}
		query.ServiceName = value.Value
		return nil
	case ref.IsField(expression.LevelSpan, expression.SpanFieldName):
		if predicate.Op != expression.OpEq {
			return errUnsupportedOperatorOn(predicate.Op, ref)
		}
		if query.OperationName != "" {
			return errRepeatedPredicateOn(ref)
		}
		query.OperationName = value.Value
		return nil
	case ref.IsField(expression.LevelSpan, expression.SpanFieldDuration):
		return applyDurationBound(query, predicate.Op, ref, value)
	default:
		return fmt.Errorf("%w: it does not support the built-in field %q of the %q level",
			ErrFilterUnsupported, ref.Name, ref.Level)
	}
}

func applyAttribute(query *TraceQueryParams, op expression.Operator, ref *expression.Reference, value *expression.Scalar) error {
	if op != expression.OpEq {
		return errUnsupportedOperatorOn(op, ref)
	}
	if !legacyFilterLevels.SupportsLevel(ref.Level) {
		return fmt.Errorf("%w: it does not index the %q level", ErrFilterUnsupported, ref.Level)
	}
	if _, ok := query.Attributes.Get(ref.Name); ok {
		return errRepeatedPredicateOn(ref)
	}
	query.Attributes.PutStr(ref.Name, value.Value)
	return nil
}

func applyDurationBound(query *TraceQueryParams, op expression.Operator, ref *expression.Reference, value *expression.Scalar) error {
	duration, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf(`%w: %q is not a duration such as "2s"`, ErrFilterInvalid, value.Value)
	}
	switch op {
	case expression.OpGte:
		if query.DurationMin != 0 {
			return errRepeatedPredicateOn(ref)
		}
		query.DurationMin = duration
	case expression.OpLte:
		if query.DurationMax != 0 {
			return errRepeatedPredicateOn(ref)
		}
		query.DurationMax = duration
	default:
		return errUnsupportedOperatorOn(op, ref)
	}
	return nil
}

// refAndConstant splits a predicate into the value it reads off the span and the constant
// it compares against. A legacy field holds a key and one value, so a predicate comparing
// two references, or testing membership of a list, has nowhere to go.
func refAndConstant(predicate *expression.Call) (*expression.Reference, *expression.Scalar, error) {
	if len(predicate.Args) != 2 {
		return nil, nil, errUnsupportedOperator(predicate.Op)
	}
	ref, refOK := predicate.Args[0].(*expression.Reference)
	value, valueOK := predicate.Args[1].(*expression.Scalar)
	if !refOK || !valueOK {
		return nil, nil, fmt.Errorf("%w: it compares a reference against a constant only", ErrFilterUnsupported)
	}
	return ref, value, nil
}

func errUnsupportedOperator(op expression.Operator) error {
	return fmt.Errorf("%w: it does not support the operator %q", ErrFilterUnsupported, op)
}

func errUnsupportedOperatorOn(op expression.Operator, ref *expression.Reference) error {
	return fmt.Errorf("%w: it does not support the operator %q on %q", ErrFilterUnsupported, op, ref.Name)
}

func errRepeatedPredicateOn(ref *expression.Reference) error {
	return fmt.Errorf("%w: it can carry only one predicate on %q", ErrFilterUnsupported, ref.Name)
}
