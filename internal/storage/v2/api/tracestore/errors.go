// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
)

// This file holds every error the storage API defines for refusing a search; the query service
// builds its own refusals for the gates and capabilities it checks before dispatch with the same
// constructors. A refusal belongs to one of two families, and each family has one root
// (ADR-013): ErrInvalidQuery for a query that is malformed on its own terms, so the caller must
// change it wherever it is sent, and errors.ErrUnsupported for a well-formed query that this
// backend lacks a capability to serve, which is also what a Reader returns when it lacks a
// method altogether. The API layers answer InvalidArgument / HTTP 400 to the first family and
// Unimplemented / HTTP 501 to the second, and decide by the root alone. Every Unsupported
// sentinel a Reader can return also has an ErrorInfo reason in status.go, because the query
// service reads a bare errors.ErrUnsupported from a Reader as a missing method and falls back
// to another one.

// ErrInvalidQuery is the root of every refusal of a query that is malformed on its own terms.
var ErrInvalidQuery = errors.New("invalid query")

// refusal is a sentinel that belongs to one family through Unwrap without repeating the root's
// text in its message, so a caller reads "invalid pagination" rather than "invalid pagination:
// invalid query". It is handed out by pointer so that two sentinels with the same wording stay
// distinct under errors.Is.
type refusal struct {
	msg  string
	root error
}

func (r *refusal) Error() string { return r.msg }
func (r *refusal) Unwrap() error { return r.root }

// InvalidQuery returns a sentinel for a query that is malformed wherever it is sent; it matches
// ErrInvalidQuery.
func InvalidQuery(msg string) error {
	return &refusal{msg: msg, root: ErrInvalidQuery}
}

// Unsupported returns a sentinel for a query that this backend lacks a capability to serve; it
// matches errors.ErrUnsupported.
func Unsupported(msg string) error {
	return &refusal{msg: msg, root: errors.ErrUnsupported}
}

// ErrFilterInvalid is returned for a query filter whose value does not fit the field it
// compares — the kind of mistake a structural check cannot catch, because the filter AST
// deliberately does not carry types (RFC 0005 §6.1).
var ErrFilterInvalid = InvalidQuery("invalid query filter")

// ErrFilterUnsupported is returned for a well-formed query filter that the storage cannot
// serve — a level it does not index, an operator it has not implemented, or a boolean
// structure a flat index cannot evaluate (RFC 0005 §7). The query is refused rather than
// approximated, so a caller never reads a narrower answer as the whole one. The query
// service returns it for the limits a Reader declared through FilterCapabilities, and a
// Reader returns it for the ones that declaration is too coarse to express — a built-in
// field of a level it serves but does not store, or an operator it serves on some
// references and not others.
var ErrFilterUnsupported = Unsupported("this storage backend cannot serve this query filter")

// ErrSpanOrderInvalid is returned for an ordering that is malformed on its own terms,
// independent of any backend (RFC 0016 §6.3).
var ErrSpanOrderInvalid = InvalidQuery("invalid span ordering")

// ErrSpanOrderUnsupported is returned for a valid explicit ordering that this backend does
// not declare (RFC 0016 §6.5).
var ErrSpanOrderUnsupported = Unsupported("unsupported span ordering")

// ErrPaginationInvalid is returned for a query whose Pagination is malformed on its own
// terms, independent of any backend: one that also sets SearchDepth, since the two bounds
// have no single honest meaning together, or one that leaves PageSize at zero, since a
// Pagination with no page size does not describe a page (RFC 0014 §4). A Reader returns it
// for a PageToken it did not produce or produced for a different query (RFC 0014 §3.2).
var ErrPaginationInvalid = InvalidQuery("invalid pagination")

// ErrPaginationUnsupported is returned for a query carrying a Pagination.PageToken to a
// Reader whose SearchCapabilities.Paginated is false. The query is refused rather than
// treated as a new search, because a Reader that cannot paginate cannot have minted the
// token, so honoring it as if it started a fresh search would silently reinterpret what
// the caller sent (RFC 0014 §6.2).
var ErrPaginationUnsupported = Unsupported("this storage backend cannot resume a paginated search")

// ErrPaginationUnsupportedByFindTraces is returned for a FindTraces query that carries
// Pagination. FindTraces streams whole traces with no field to carry a continuation token,
// so honoring the request would accept a paging request and never hand back a cursor,
// leaving the caller unable to tell a bounded page from the last one (RFC 0014 §4). No
// deployment can serve it, so it is a malformed query rather than a missing capability.
var ErrPaginationUnsupportedByFindTraces = InvalidQuery("FindTraces cannot be paginated: its response has no field to carry a continuation token")
