// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// StructuredFiltersGate admits the RFC 0005 structured query filter. It is off by default
// because the filter AST is a public API still moving through that RFC's milestones, so a
// deployment that has not opted in behaves exactly as it did before the filter existed.
//
// It admits the filter into the query path; whether a backend evaluates one natively is a
// separate switch, named jaeger.<backend>.structuredFilters — this gate's leaf with the backend
// in place of "query" — because the two stabilize on different schedules.
var StructuredFiltersGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.structuredFilters",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0005 structured query filter on trace search. The filter AST is not yet "+
			"stable, so a query that carries one is refused while this is disabled. This admits "+
			"filters into the query path only; whether a storage backend evaluates one natively is "+
			"gated separately, by jaeger.<backend>.structuredFilters.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0005-structured-query-filters.md"),
)

// ErrFilterDisabled is returned for a query carrying a filter to a deployment that has not
// enabled StructuredFiltersGate. The query is refused rather than served with the filter
// ignored, because dropping a predicate would answer with every trace in the time range.
var ErrFilterDisabled = errors.New("the structured query filter is disabled")

// IsBadRequest reports whether err means the caller must change the query, either
// because its shape is wrong or because this deployment's storage cannot serve it.
// Either way it is the caller's problem, so the API layers answer InvalidArgument /
// HTTP 400 rather than reporting a server fault.
func IsBadRequest(err error) bool {
	return errors.Is(err, ErrServiceNameRequired) ||
		errors.Is(err, ErrFilterDisabled) ||
		errors.Is(err, tracestore.ErrFilterUnsupported) ||
		errors.Is(err, tracestore.ErrFilterInvalid)
}

// prepareFilteredQuery gives the reader whichever of the two filtering models it declared it
// can evaluate, and answers only that question — the caller has already established that the
// request is one this deployment accepts. A reader that declares filter support gets the filter
// itself, once every level and operator it uses is one that reader listed. A reader that
// declares none — every backend until per-backend routing lands, and any remote plugin that
// predates the capability — gets the filter rewritten into the legacy predicate fields, which
// carry the equalities and inclusive duration bounds and nothing else
// (TraceQueryParams.ToLegacyShape).
func prepareFilteredQuery(query TraceQueryParams, caps tracestore.SearchCapabilities) (TraceQueryParams, error) {
	if caps.Filter.IsEmpty() {
		legacy, err := query.TraceQueryParams.ToLegacyShape()
		if err != nil {
			return query, err
		}
		query.TraceQueryParams = legacy
		return query, nil
	}
	return query, ensureFilterSupported(query.Filter, *caps.Filter)
}

// ensureWellFormedFilter rejects a filter that is not a well-formed tree. Decoding a filter off
// a wire does not check this, so the query service asks here on behalf of every API layer above
// it: an operator or level this build has no meaning for, or an operator given the wrong number
// or kind of arguments, would otherwise reach a backend that typically answers such a predicate
// by matching nothing rather than by refusing.
func ensureWellFormedFilter(filter *expression.Call) error {
	if err := expression.ValidateFilter(filter); err != nil {
		return fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
	}
	return nil
}

// ensureNoLegacyPredicates rejects a query that carries both a filter and one of the
// predicate fields the filter replaces. The two express the same things — a service, an
// operation name, a duration bound, a tag — so honoring both would leave the caller
// guessing which one applied.
func ensureNoLegacyPredicates(query TraceQueryParams) error {
	var set []string
	if query.ServiceName != "" {
		set = append(set, "service_name")
	}
	if query.OperationName != "" {
		set = append(set, "operation_name")
	}
	if query.DurationMin != 0 {
		set = append(set, "duration_min")
	}
	if query.DurationMax != 0 {
		set = append(set, "duration_max")
	}
	if query.Attributes != (pcommon.Map{}) && query.Attributes.Len() > 0 {
		set = append(set, "attributes")
	}
	if len(set) == 0 {
		return nil
	}
	return fmt.Errorf("%w: it cannot be combined with %v; express those predicates in the filter instead",
		tracestore.ErrFilterInvalid, set)
}

// ensureFilterSupported walks the filter and refuses the first predicate the reader did
// not declare it can evaluate.
func ensureFilterSupported(filter *expression.Call, caps tracestore.FilterCapabilities) error {
	if !caps.SupportsOperator(filter.Op) {
		return fmt.Errorf("%w: it does not support the operator %q",
			tracestore.ErrFilterUnsupported, filter.Op)
	}
	for _, arg := range filter.Args {
		switch term := arg.(type) {
		case *expression.Call:
			if err := ensureFilterSupported(term, caps); err != nil {
				return err
			}
		case *expression.Reference:
			if !caps.SupportsLevel(term.Level) {
				return fmt.Errorf("%w: it does not index the %q level", tracestore.ErrFilterUnsupported, term.Level)
			}
		default:
			// A constant carries nothing a reader has to support.
		}
	}
	return nil
}
