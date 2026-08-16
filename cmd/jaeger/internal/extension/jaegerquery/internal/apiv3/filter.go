// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"fmt"

	"go.opentelemetry.io/collector/featuregate"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
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

// errStructuredFiltersDisabled reports a request that carries a filter to a deployment that
// has not enabled the gate. Each entry point raises it for itself, before reading the filter,
// because admitting the request is its concern and not the conversion's. The request is
// refused rather than served with the filter ignored: dropping the predicate would answer
// with every trace in the time range, more than the caller asked for.
func errStructuredFiltersDisabled() error {
	return fmt.Errorf("the structured query filter is disabled: enable the %q feature gate to use it",
		structuredFiltersGate.ID())
}

// toFilter converts the filter of an api_v3 request into the filter AST and checks that it is
// well formed. A request without a filter converts to nil.
func toFilter(filter *expressionproto.Call) (*expression.Call, error) {
	call, err := expressionproto.ToFilter(filter)
	if err != nil || call == nil {
		return nil, err
	}
	if err := expression.ValidateFilter(call); err != nil {
		return nil, err
	}
	return call, nil
}
