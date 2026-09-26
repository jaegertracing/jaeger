// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// This file turns the request a caller sends into the query a reader receives. The two are
// different types on purpose, so every default, clamp and check lands here and never on the
// caller's request; conversion_test.go checks by reflection that every field is carried across.

// toReaderQuery checks the fields every trace search carries whichever filtering model it uses,
// and returns the query in the reader's shape with DefaultSearchDepth applied where the caller
// left the bound unset and an oversized page clamped rather than refused (RFC 0014 §4, AIP-158).
// The API handlers only translate their wire shape into the request; what a query must satisfy
// is decided here, once.
func (q TraceQueryParams) toReaderQuery() (tracestore.TraceQueryParams, error) {
	query := tracestore.TraceQueryParams{
		ServiceName:   q.ServiceName,
		OperationName: q.OperationName,
		Attributes:    q.Attributes,
		StartTimeMin:  q.StartTimeMin,
		StartTimeMax:  q.StartTimeMax,
		DurationMin:   q.DurationMin,
		DurationMax:   q.DurationMax,
		SearchDepth:   q.SearchDepth,
		Filter:        q.Filter,
	}
	if q.StartTimeMin.IsZero() || q.StartTimeMax.IsZero() {
		return query, fmt.Errorf("%w: min and max start time are required", ErrQueryInvalid)
	}
	if !q.StartTimeMin.Before(q.StartTimeMax) {
		return query, fmt.Errorf("%w: min start time must be before max start time", ErrQueryInvalid)
	}
	if q.DurationMin < 0 || q.DurationMax < 0 {
		return query, fmt.Errorf("%w: min and max duration cannot be negative", ErrQueryInvalid)
	}
	if q.DurationMin > 0 && q.DurationMax > 0 && q.DurationMax < q.DurationMin {
		return query, fmt.Errorf("%w: max duration cannot be less than min duration", ErrQueryInvalid)
	}
	if q.SearchDepth < 0 || q.SearchDepth > tracestore.MaxSearchDepth {
		return query, fmt.Errorf("%w: search depth must be in [0, %d]", ErrQueryInvalid, tracestore.MaxSearchDepth)
	}
	if q.Pagination == nil {
		if q.SearchDepth == 0 {
			query.SearchDepth = DefaultSearchDepth
		}
		return query, nil
	}
	if !PaginationGate.IsEnabled() {
		return query, fmt.Errorf("%w: enable the %q feature gate to use it",
			ErrPaginationDisabled, PaginationGate.ID())
	}
	// A page size replaces the search depth rather than falling back to it (RFC 0014 §4).
	if q.SearchDepth != 0 {
		return query, fmt.Errorf("%w: it cannot be combined with search depth",
			tracestore.ErrPaginationInvalid)
	}
	if q.Pagination.PageSize <= 0 {
		return query, fmt.Errorf("%w: page size is required whenever pagination is present",
			tracestore.ErrPaginationInvalid)
	}
	query.Pagination = &tracestore.Pagination{
		PageSize:  min(q.Pagination.PageSize, tracestore.MaxPageSize),
		PageToken: q.Pagination.PageToken,
	}
	return query, nil
}

// toReaderQuery is the span-search counterpart of TraceQueryParams.toReaderQuery: it checks the
// time window, applies the pagination gate, and fills in or clamps the page size. A page token is
// what makes the request a paginated one, and that is what the feature gate governs. The page
// size is only the bound (RFC 0016 §6): unset means DefaultPageSize, as an omitted size does on
// Elasticsearch, and an oversized one is clamped rather than refused (RFC 0014 §4). The filter
// and the reader's capabilities are prepareSpanSearchQuery's, since they depend on the backend.
func (q SpanQueryParams) toReaderQuery() (tracestore.SpanQueryParams, error) {
	query := tracestore.SpanQueryParams{
		StartTimeMin: q.StartTimeMin,
		StartTimeMax: q.StartTimeMax,
		Filter:       q.Filter,
		Pagination: tracestore.Pagination{
			PageSize:  q.Pagination.PageSize,
			PageToken: q.Pagination.PageToken,
		},
	}
	if q.StartTimeMin.IsZero() || q.StartTimeMax.IsZero() {
		return query, fmt.Errorf("%w: start_time_min and start_time_max are required", ErrQueryInvalid)
	}
	if !q.StartTimeMin.Before(q.StartTimeMax) {
		return query, fmt.Errorf("%w: start_time_min must be before start_time_max", ErrQueryInvalid)
	}
	if q.Pagination.PageToken != "" && !PaginationGate.IsEnabled() {
		return query, fmt.Errorf("%w: enable the %q feature gate to use it",
			ErrPaginationDisabled, PaginationGate.ID())
	}
	if q.Pagination.PageSize == 0 {
		query.Pagination.PageSize = DefaultPageSize
	}
	query.Pagination.PageSize = min(query.Pagination.PageSize, tracestore.MaxPageSize)
	return query, nil
}
