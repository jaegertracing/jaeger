// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// structuredFiltersGate admits the RFC 0005 structured query filter on trace search. It is
// off by default because the filter AST is a public API still moving through that RFC's
// milestones, so a deployment that has not opted in behaves exactly as it did before the
// filter existed. It sits at the api_v3 edge rather than deeper, so that while it is off no
// filter enters the system and nothing downstream of it can run.
//
// It admits the filter into the query path; whether a backend evaluates one natively is a
// separate switch, named jaeger.<backend>.structuredFilters — this gate's leaf with the
// backend in place of "query" — because the two stabilize on different schedules. With this
// gate on and a backend's off, that backend declares no FilterCapabilities, so the query
// service rewrites the filter into the legacy predicate fields instead of sending it down.
var structuredFiltersGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.structuredFilters",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0005 structured query filter on api_v3 trace search. The filter AST is "+
			"not yet stable, so a request that carries one is refused while this is disabled. This "+
			"admits filters into the query path only; whether a storage backend evaluates one "+
			"natively is gated separately, by jaeger.<backend>.structuredFilters.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0005-structured-query-filters.md"),
)

// checkStructuredFiltersEnabled refuses a request that carries a filter while the gate is
// off. It refuses rather than ignoring the filter, because dropping a predicate would answer
// with every trace in the time range — more than the caller asked for.
func checkStructuredFiltersEnabled() error {
	if structuredFiltersGate.IsEnabled() {
		return nil
	}
	return fmt.Errorf("the structured query filter is disabled: enable the %q feature gate to use it",
		structuredFiltersGate.ID())
}

// toStorageFilter converts the filter of an api_v3 request into the storage filter and
// checks that it is well formed. A request without a filter converts to nil.
func toStorageFilter(filter *api_v3.Call) (*tracestore.Call, error) {
	if filter == nil {
		return nil, nil
	}
	if err := checkStructuredFiltersEnabled(); err != nil {
		return nil, err
	}
	call, err := toStorageCall(filter)
	if err != nil {
		return nil, err
	}
	if err := tracestore.ValidateFilter(call); err != nil {
		return nil, err
	}
	return call, nil
}

func toStorageCall(call *api_v3.Call) (*tracestore.Call, error) {
	args := make([]tracestore.Expression, 0, len(call.GetArgs()))
	for _, arg := range call.GetArgs() {
		expr, err := toStorageExpression(arg)
		if err != nil {
			return nil, err
		}
		args = append(args, expr)
	}
	return &tracestore.Call{
		Op:   tracestore.Operator(call.GetOp()),
		Args: args,
	}, nil
}

func toStorageExpression(expr *api_v3.Expression) (tracestore.Expression, error) {
	switch term := expr.GetTerm().(type) {
	case *api_v3.Expression_Ref:
		return &tracestore.Reference{
			Name:  term.Ref.GetName(),
			Level: tracestore.Level(term.Ref.GetLevel()),
			Attr:  term.Ref.GetAttr(),
		}, nil
	case *api_v3.Expression_Scalar:
		return &tracestore.Scalar{
			Value: term.Scalar.GetValue(),
			Type:  tracestore.ValueType(term.Scalar.GetType()),
		}, nil
	case *api_v3.Expression_List:
		return &tracestore.List{
			Values: term.List.GetValues(),
			Type:   tracestore.ValueType(term.List.GetType()),
		}, nil
	case *api_v3.Expression_Call:
		nested, err := toStorageCall(term.Call)
		if err != nil {
			return nil, err
		}
		return nested, nil
	default:
		return nil, errors.New("filter argument is empty: exactly one of ref, scalar, list or call must be set")
	}
}
