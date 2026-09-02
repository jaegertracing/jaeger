// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"
)

// PaginationGate admits the RFC 0014 Pagination request field into the query path. It does not
// deliver working pagination on its own: no Reader yields a next_page_token yet — FindTraceIDs
// and FindTraceSummaries still return unpaginated batches — so enabling this gate only lets a
// request carrying Pagination reach admission instead of being refused outright; the response
// side (RFC 0014 §5, M2's remaining exit bar) lands separately.
var PaginationGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.pagination",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0014 Pagination field on trace search. A query carrying Pagination is "+
			"refused while this is disabled. This is the request-side admission only: no Reader "+
			"returns a continuation token yet, so enabling it does not yet make search results "+
			"resumable end to end.",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0014-search-result-pagination.md"),
)

// ErrPaginationDisabled is returned for a query carrying Pagination to a deployment that has
// not enabled PaginationGate. The query is refused rather than served as an unpaginated
// search, so a caller that expects a next_page_token is never silently given none.
var ErrPaginationDisabled = errors.New("pagination is disabled")
