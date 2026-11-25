// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// StructuredFiltersGate stays registered so that a deployment that names it in --feature-gates
// still starts. It is Stable and cannot be disabled, so the query service admits the RFC 0005
// structured query filter without consulting it. The gate ID is removed in v2.26.0.
//
// Whether a backend evaluates a filter natively is a separate switch, named
// jaeger.<backend>.structuredFilters — this gate's leaf with the backend in place of "query" —
// because the two stabilize on different schedules.
var StructuredFiltersGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.structuredFilters",
	featuregate.StageStable,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterToVersion("v2.26.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0005 structured query filter on trace search. This admits filters into "+
			"the query path only; whether a storage backend evaluates one natively is gated "+
			"separately, by jaeger.<backend>.structuredFilters.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0005-structured-query-filters.md"),
)

// queryToReaderCapabilities returns the query in the shape the reader declared it can serve, immediately
// before dispatch.
//
// A reader that cannot paginate has no field to read PageSize from, so PageSize becomes the
// SearchDepth and Pagination is cleared, which keeps the search bounded. A PageToken is never
// folded: a reader that cannot paginate cannot have minted it, so the query is refused rather
// than restarted as a new search (RFC 0014 §6.2).
//
// A reader that declares filter support gets the filter itself, once every level and operator
// it uses is one the reader listed. A reader that declares none gets the filter rewritten into
// the legacy predicate fields (ToLegacyShape), or a refusal where they cannot carry it.
func queryToReaderCapabilities(
	query tracestore.TraceQueryParams,
	caps tracestore.SearchCapabilities,
) (tracestore.TraceQueryParams, error) {
	if !caps.Paginated {
		var err error
		if query, err = query.ReplacePaginationWithDepth(); err != nil {
			return tracestore.TraceQueryParams{}, err
		}
	}
	if query.Filter == nil {
		return query, nil
	}
	if caps.Filter.IsEmpty() {
		return query.ToLegacyShape()
	}
	return query, caps.Filter.EnsureSupported(query.Filter)
}
