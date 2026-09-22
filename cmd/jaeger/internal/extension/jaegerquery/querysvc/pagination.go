// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// PaginationGate admits the RFC 0014 Pagination field on a trace search. No Reader returns a
// continuation token yet, so enabling it does not make results resumable end to end.
var PaginationGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.pagination",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0014 Pagination field on a trace search. No storage backend returns a "+
			"continuation token yet, so enabling it does not make results resumable end to end.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0014-search-result-pagination.md"),
)

// ErrPaginationDisabled is returned for a query carrying Pagination while PaginationGate is off.
var ErrPaginationDisabled = errors.New("pagination is disabled")

// paginationForCapabilities is the Pagination half of forCapabilities. A reader that declares
// Paginated gets Pagination as sent. One that does not has no field to read PageSize from, so
// PageSize is folded into SearchDepth and Pagination is cleared, which keeps the search bounded.
// A PageToken is never folded: a reader that cannot paginate cannot have minted it, so the query
// is refused (RFC 0014 §6.2) rather than restarted as a new search.
func paginationForCapabilities(
	query tracestore.TraceQueryParams,
	caps tracestore.SearchCapabilities,
) (tracestore.TraceQueryParams, error) {
	if query.Pagination == (tracestore.Pagination{}) || caps.Paginated {
		return query, nil
	}
	if query.Pagination.PageToken != "" {
		return tracestore.TraceQueryParams{}, tracestore.ErrPaginationUnsupported
	}
	query.SearchDepth = query.Pagination.PageSize
	query.Pagination = tracestore.Pagination{}
	return query, nil
}
