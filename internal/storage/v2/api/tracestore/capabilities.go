// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"slices"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// ErrFilterUnsupported is returned for a well-formed query filter that the storage cannot
// serve — a level it does not index, an operator it has not implemented, or a boolean
// structure a flat index cannot evaluate (RFC 0005 §7). The query is refused rather than
// approximated, so a caller never reads a narrower answer as the whole one. The query
// service returns it for the limits a Reader declared through FilterCapabilities, and a
// Reader returns it for the ones that declaration is too coarse to express — a built-in
// field of a level it serves but does not store, or an operator it serves on some
// references and not others.
var ErrFilterUnsupported = errors.New("this storage backend cannot serve this query filter")

// ErrFilterInvalid is returned for a query filter whose value does not fit the field it
// compares — the kind of mistake a structural check cannot catch, because the filter AST
// deliberately does not carry types (RFC 0005 §6.1).
var ErrFilterInvalid = errors.New("invalid query filter")

// ErrPaginationUnsupported is returned for a query carrying a Pagination.PageToken to a
// Reader whose SearchCapabilities.Paginated is false. The query is refused rather than
// treated as a new search, because a Reader that cannot paginate cannot have minted the
// token, so honoring it as if it started a fresh search would silently reinterpret what
// the caller sent (RFC 0014 §6.2).
var ErrPaginationUnsupported = errors.New("this storage backend cannot resume a paginated search")

// ErrPaginationInvalid is returned for a query whose Pagination is malformed on its own
// terms, independent of any backend: one that also sets SearchDepth, since the two bounds
// have no single honest meaning together, or one that leaves PageSize at zero, since a
// Pagination with no page size does not describe a page (RFC 0014 §4).
var ErrPaginationInvalid = errors.New("invalid pagination")

// ErrPaginationUnsupportedByFindTraces is returned for a FindTraces query that carries
// Pagination. FindTraces streams whole traces with no field to carry a continuation token,
// so honoring the request would accept a paging request and never hand back a cursor,
// leaving the caller unable to tell a bounded page from the last one (RFC 0014 §4).
var ErrPaginationUnsupportedByFindTraces = errors.New("FindTraces cannot be paginated: its response has no field to carry a continuation token")

// SearchCapabilities describes how a Reader's search methods behave where backends
// differ: which TraceQueryParams fields may be omitted, which are honored exactly
// rather than approximated, and which combinations a backend cannot serve. Its zero
// value is the least capable reader, so a field added here leaves every existing
// implementation declaring the new capability unsupported.
//
// Fields to expect over time, each of which is a real divergence today:
//
//   - Whether SearchDepth is an exact limit or a hint. jaeger.api_v3's
//     TraceQueryParameters warns of search_depth that "some implementations might not
//     support precise limits", so a caller cannot tell whether a short result set means
//     that there are no more matches or that the backend stopped early.
//   - Which duration-query combinations hold. Cassandra reads DurationMin/DurationMax
//     from a separate duration_index table, and it cannot combine that table with the
//     tag index in one query, so it rejects a search that uses both
//     (docs/adr/001-cassandra-find-traces-duration.md). The API layer stopped rejecting
//     the combination in https://github.com/jaegertracing/jaeger/issues/1047, which did
//     not remove the storage limitation.
type SearchCapabilities struct {
	// WithoutServiceName is true when FindTraces, FindTraceIDs and FindTraceSummaries
	// accept a TraceQueryParams whose ServiceName is empty and read it as "any
	// service", rather than as an error or an empty result.
	WithoutServiceName bool

	// SameSpanConjunction is true when a conjunction — several entries in the Attributes
	// map, or an OpAnd in Filter — is satisfied within a single span. False means the
	// backend may satisfy different conjuncts from different spans of the same trace, as
	// a flat inverted index that intersects at trace granularity does. Callers report
	// this looser scoping rather than refusing the query.
	SameSpanConjunction bool

	// Filter is how much of TraceQueryParams.Filter the reader evaluates. A nil Filter
	// means none of it: the reader serves only the other, legacy fields, so a caller with
	// a filter to run must express it in those fields or refuse the query.
	Filter *FilterCapabilities

	// Paginated is true when FindTraceIDs and FindTraceSummaries honor
	// TraceQueryParams.Pagination and let a caller resume a search past its first page
	// (RFC 0014). False, the zero value, means the reader cannot paginate: the query
	// service serves a single page capped at Pagination.PageSize or SearchDepth and
	// rejects a query that carries a PageToken, since a reader that cannot paginate
	// cannot have minted a valid one.
	Paginated bool
}

// FilterCapabilities declares how much of a structured filter a Reader evaluates, by naming
// the levels and operators it serves. Nothing is implicit: a Reader opts in only by naming
// something, so an empty FilterCapabilities says the same as no FilterCapabilities at all —
// see IsEmpty, which is what callers ask rather than comparing against nil.
type FilterCapabilities struct {
	// Levels are the levels a reference may name. Empty means the Reader can serve only
	// unqualified references.
	Levels []expression.Level
	// Operators are the operators the Reader evaluates. The boolean combinators are listed
	// here like any other operator: a flat inverted index declares OpAnd and omits OpOr and
	// OpNot, which is what confines it to the conjunctive subset. Nesting is not declared
	// separately, because OpAnd is associative and a caller flattens it before asking.
	Operators []expression.Operator
}

// IsEmpty reports whether the Reader declared no part of a structured filter, and so evaluates
// none of it. A nil FilterCapabilities and an empty one both answer true, so a caller never has
// to tell "declared nothing" from "did not declare", and a Reader cannot half-opt-in.
func (c *FilterCapabilities) IsEmpty() bool {
	return c == nil || (len(c.Levels) == 0 && len(c.Operators) == 0)
}

// SupportsLevel reports whether a reference at the given level may reach the Reader.
// An unqualified reference always may.
func (c FilterCapabilities) SupportsLevel(level expression.Level) bool {
	if level == "" {
		return true
	}
	return slices.Contains(c.Levels, level)
}

// SupportsOperator reports whether the Reader evaluates the given operator.
func (c FilterCapabilities) SupportsOperator(op expression.Operator) bool {
	return slices.Contains(c.Operators, op)
}
