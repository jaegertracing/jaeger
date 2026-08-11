// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// legacyFilterLevels are the attribute levels every backend indexes, and so the levels a
// predicate can be widened to an unqualified tag search over. Instrumentation attributes
// are folded into span tags and link attributes are not stored at all, so a predicate at
// either level cannot be answered by widening (RFC 0005 §1.6).
var legacyFilterLevels = tracestore.FilterCapabilities{
	Levels: []tracestore.Level{
		tracestore.LevelSpan,
		tracestore.LevelResource,
		tracestore.LevelEvent,
	},
}

// rewriteFilterAsLegacyFields expresses a filter in the predicate fields every backend
// already serves, so a filter query returns the traces a legacy query would. It accepts a
// flat conjunction of equalities over the attribute levels every backend indexes — a
// backend that searches all of them answers with a superset — plus the service, the
// operation name, and the inclusive duration bounds. Everything else is refused: widening
// a level a backend never indexed, or reading `gt` as the inclusive bound the legacy field
// carries, would answer a different question than the one asked.
func rewriteFilterAsLegacyFields(query TraceQueryParams) (TraceQueryParams, error) {
	predicates, err := flatConjuncts(query.Filter)
	if err != nil {
		return query, err
	}
	rewritten := query
	rewritten.Filter = nil
	// A fresh map, because the one on query is shared with the caller and prepareFilter has
	// already established that it is empty.
	rewritten.Attributes = pcommon.NewMap()
	for _, predicate := range predicates {
		if err := applyAsLegacyField(&rewritten, predicate); err != nil {
			return query, err
		}
	}
	return rewritten, nil
}

// flatConjuncts returns the predicates of a flat conjunction: the arguments of a
// top-level `and`, or the filter itself when it is a single predicate.
func flatConjuncts(filter *tracestore.Call) ([]*tracestore.Call, error) {
	if filter.Op != tracestore.OpAnd {
		if isBoolean(filter.Op) {
			return nil, errUnsupportedOperator(filter.Op)
		}
		return []*tracestore.Call{filter}, nil
	}
	conjuncts := make([]*tracestore.Call, 0, len(filter.Args))
	for _, arg := range filter.Args {
		predicate, ok := arg.(*tracestore.Call)
		if !ok {
			return nil, fmt.Errorf("%w: it evaluates a conjunction of predicates only", ErrFilterUnsupported)
		}
		if isBoolean(predicate.Op) {
			return nil, errNestedBoolean()
		}
		conjuncts = append(conjuncts, predicate)
	}
	return conjuncts, nil
}

// applyAsLegacyField writes one predicate into the legacy field that carries it.
func applyAsLegacyField(query *TraceQueryParams, predicate *tracestore.Call) error {
	ref, value, err := refAndConstant(predicate)
	if err != nil {
		return err
	}
	switch {
	case ref.IsAttribute():
		return applyAttribute(query, predicate.Op, ref, value)
	case ref.IsField(tracestore.ResourceService):
		if predicate.Op != tracestore.OpEq {
			return errUnsupportedOperatorOn(predicate.Op, ref)
		}
		if query.ServiceName != "" {
			return errRepeatedPredicateOn(ref)
		}
		query.ServiceName = value.Value
		return nil
	case ref.IsField(tracestore.SpanName):
		if predicate.Op != tracestore.OpEq {
			return errUnsupportedOperatorOn(predicate.Op, ref)
		}
		if query.OperationName != "" {
			return errRepeatedPredicateOn(ref)
		}
		query.OperationName = value.Value
		return nil
	case ref.IsField(tracestore.SpanDuration):
		return applyDurationBound(query, predicate.Op, ref, value)
	default:
		return fmt.Errorf("%w: it does not support the built-in field %q of the %q level",
			ErrFilterUnsupported, ref.Name, ref.Level)
	}
}

func applyAttribute(query *TraceQueryParams, op tracestore.Operator, ref *tracestore.Reference, value *tracestore.Scalar) error {
	if op != tracestore.OpEq {
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

func applyDurationBound(query *TraceQueryParams, op tracestore.Operator, ref *tracestore.Reference, value *tracestore.Scalar) error {
	duration, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf(`%w: %q is not a duration such as "2s"`, ErrFilterInvalid, value.Value)
	}
	switch op {
	case tracestore.OpGte:
		if query.DurationMin != 0 {
			return errRepeatedPredicateOn(ref)
		}
		query.DurationMin = duration
	case tracestore.OpLte:
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
func refAndConstant(predicate *tracestore.Call) (*tracestore.Reference, *tracestore.Scalar, error) {
	if len(predicate.Args) != 2 {
		return nil, nil, errUnsupportedOperator(predicate.Op)
	}
	ref, refOK := predicate.Args[0].(*tracestore.Reference)
	value, valueOK := predicate.Args[1].(*tracestore.Scalar)
	if !refOK || !valueOK {
		return nil, nil, fmt.Errorf("%w: it compares a reference against a constant only", ErrFilterUnsupported)
	}
	return ref, value, nil
}

func errUnsupportedOperatorOn(op tracestore.Operator, ref *tracestore.Reference) error {
	return fmt.Errorf("%w: it does not support the operator %q on %q", ErrFilterUnsupported, op, ref.Name)
}

func errRepeatedPredicateOn(ref *tracestore.Reference) error {
	return fmt.Errorf("%w: it can carry only one predicate on %q", ErrFilterUnsupported, ref.Name)
}
