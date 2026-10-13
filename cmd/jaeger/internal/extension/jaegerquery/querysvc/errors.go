// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// This file holds every error the query service defines. The refusals are built with the
// tracestore constructors so they join one of the two families of ADR-013: each of them is a
// capability this deployment lacks, so it matches errors.ErrUnsupported and the API layers
// answer Unimplemented / HTTP 501. The malformed-query family has no sentinel of its own here;
// the envelope checks wrap tracestore.ErrInvalidQuery directly. The remaining errors are
// deployment faults, which belong to neither family and reach the client as a server error.

// ErrServiceNameRequired is returned for a search that omits the service name against a
// backend whose reader does not accept one (RFC 0013 §3.3). It names the backend's
// limitation rather than the missing field, because the same query is valid elsewhere.
var ErrServiceNameRequired = tracestore.Unsupported(
	"this storage backend requires a service name to search; searching all services is not supported",
)

// ErrSpanSearchUnsupported is returned for a span search against a backend whose reader does
// not declare SpanSearch (RFC 0016 §4.5). It names the backend's limitation, because the same
// query is valid elsewhere. The interceptor package has a sentinel of the same name for an
// interceptor with no span-search policy; that one is a deployment fault, not a refusal.
var ErrSpanSearchUnsupported = tracestore.Unsupported("this storage backend does not declare span search support")

// ErrFilterDisabled is returned for a query carrying a filter to a deployment that has not
// enabled StructuredFiltersGate. The query is refused rather than served with the filter
// ignored, because dropping a predicate would answer with every trace in the time range. The
// same query is valid on a deployment with the gate on.
var ErrFilterDisabled = tracestore.Unsupported("the structured query filter is disabled")

// ErrPaginationDisabled is returned for a query carrying Pagination while PaginationGate is off.
// The same query is valid on a deployment with the gate on.
var ErrPaginationDisabled = tracestore.Unsupported("pagination is disabled")

// ErrInterceptorFilter reports that a query interceptor returned a filter jaeger-query will not
// send to storage. It is deliberately in neither refusal family: the caller's request was fine,
// and the fault is in the extension this deployment configured.
var ErrInterceptorFilter = errors.New("query interceptor returned an invalid filter")

// errInterceptorDroppedFilter is the one interceptor mistake that fails open: a search that had
// predicates and leaves with none asks for everything in the time range.
var errInterceptorDroppedFilter = fmt.Errorf("%w: it returned no filter for a query that had predicates, which "+
	"would widen the search to everything in the time range", ErrInterceptorFilter)

// errNoArchiveSpanStorage reports a request for archived traces on a deployment that has no
// archive storage configured.
var errNoArchiveSpanStorage = errors.New("archive span storage was not configured")
