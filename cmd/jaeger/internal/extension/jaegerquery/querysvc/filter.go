// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"

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
		errors.Is(err, tracestore.ErrFilterInvalid) ||
		errors.Is(err, ErrPaginationDisabled) ||
		errors.Is(err, tracestore.ErrPaginationUnsupported)
}
