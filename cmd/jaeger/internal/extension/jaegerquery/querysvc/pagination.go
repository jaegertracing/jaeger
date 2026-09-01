// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"
)

// PaginationGate admits the RFC 0014 Pagination request field. It is off by default because
// no storage backend can honor a page token yet (SearchCapabilities.Paginated is false
// everywhere), so a deployment that has not opted in behaves exactly as it did before
// pagination existed.
//
// It admits Pagination into the query path; whether a backend actually paginates is a
// separate signal, SearchCapabilities.Paginated, because the two stabilize on different
// schedules — this gate governs when jaeger-query starts accepting the request shape, not
// when any particular backend can serve it.
var PaginationGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.query.pagination",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.21.0"),
	featuregate.WithRegisterDescription(
		"Accepts the RFC 0014 Pagination field on trace search. This admits pagination "+
			"requests into the query path only; no storage backend can honor a page token "+
			"yet, so a request carrying one is refused with InvalidArgument regardless of "+
			"this gate (RFC 0014 §6.2).",
	),
	featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/blob/main/docs/rfc/0014-search-result-pagination.md"),
)

// ErrPaginationDisabled is returned for a query carrying Pagination to a deployment that has
// not enabled PaginationGate. The query is refused rather than served as an unpaginated
// search, so a caller that expects a next_page_token is never silently given none.
var ErrPaginationDisabled = errors.New("pagination is disabled")
