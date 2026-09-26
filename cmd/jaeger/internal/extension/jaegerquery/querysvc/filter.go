// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// StructuredFiltersGate admits the RFC 0005 structured query filter. It is on by default, and
// a deployment that disables it behaves exactly as it did before the filter existed, so an
// operator can withhold the filter AST while that RFC's remaining milestones settle it.
//
// It admits the filter into the query path; whether a backend evaluates one natively is a
// separate switch, named jaeger.<backend>.structuredFilters — this gate's leaf with the backend
// in place of "query" — because the two stabilize on different schedules.
var StructuredFiltersGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.structuredFilters",
	featuregate.StageBeta,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0005 structured query filter on trace search. A query that carries one "+
			"is refused while this is disabled. This admits filters into the query path only; "+
			"whether a storage backend evaluates one natively is gated separately, by "+
			"jaeger.<backend>.structuredFilters.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0005-structured-query-filters.md"),
)

// ErrFilterDisabled is returned for a query carrying a filter to a deployment that has not
// enabled StructuredFiltersGate. The query is refused rather than served with the filter
// ignored, because dropping a predicate would answer with every trace in the time range.
var ErrFilterDisabled = errors.New("the structured query filter is disabled")

// queryToReaderShape returns the query in the shape the reader declared it can serve, immediately
// before dispatch.
//
// A reader that cannot paginate has no field to read PageSize from, so PageSize is folded into
// SearchDepth and Pagination is cleared, which keeps the search bounded. A PageToken is never
// folded: a reader that cannot paginate cannot have minted it, so the query is refused
// (RFC 0014 §6.2) rather than restarted as a new search.
//
// A reader that declares filter support gets the filter itself, once every level and operator
// it uses is one the reader listed. A reader that declares none gets the filter rewritten into
// the legacy predicate fields (ToLegacyShape), or a refusal where they cannot carry it.
func queryToReaderShape(
	query tracestore.TraceQueryParams,
	caps tracestore.SearchCapabilities,
) (tracestore.TraceQueryParams, error) {
	if query.Pagination != nil && !caps.Paginated {
		if len(query.Pagination.PageToken) != 0 {
			return tracestore.TraceQueryParams{}, tracestore.ErrPaginationUnsupported
		}
		query.SearchDepth = query.Pagination.PageSize
		query.Pagination = nil
	}
	if query.Filter == nil {
		return query, nil
	}
	if caps.Filter.IsEmpty() {
		return query.ToLegacyShape()
	}
	return query, caps.Filter.EnsureSupported(query.Filter)
}

// IsBadRequest reports whether err means the caller must change the query, either
// because its shape is wrong or because this deployment's storage cannot serve it.
// Either way it is the caller's problem, so the API layers answer InvalidArgument /
// HTTP 400 rather than reporting a server fault.
func IsBadRequest(err error) bool {
	return errors.Is(err, ErrQueryInvalid) ||
		errors.Is(err, ErrServiceNameRequired) ||
		errors.Is(err, ErrSpanSearchUnsupported) ||
		errors.Is(err, ErrFilterDisabled) ||
		errors.Is(err, tracestore.ErrFilterUnsupported) ||
		errors.Is(err, tracestore.ErrFilterInvalid) ||
		errors.Is(err, ErrPaginationDisabled) ||
		errors.Is(err, tracestore.ErrPaginationUnsupported) ||
		errors.Is(err, tracestore.ErrPaginationInvalid) ||
		errors.Is(err, tracestore.ErrPaginationUnsupportedByFindTraces)
}
