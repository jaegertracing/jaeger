// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"

	pagetoken "github.com/jaegertracing/jaeger/internal/proto/pagetoken/v1"
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

// pageTokenMinter mints the page tokens of one search and checks the one that comes back, so both
// are bound to the query as dispatched (RFC 0014 §3.2): the token a client sent is exchanged for
// the reader's cursor it wraps, and the cursor the reader returns is minted into the token the
// client receives. The zero value belongs to a search that paginates nothing, which mints no token;
// any token such a search carried was refused while the query was prepared.
type pageTokenMinter struct {
	// template is the token every page of this search is minted from, with the cursor left
	// empty; nil for a search that paginates nothing.
	template *pagetoken.PageToken
}

// resumeSpanSearch exchanges the page token in query for the reader's cursor, in place, and returns
// the minter of the reader's next cursor. A span search paginates whenever the gate is
// on; with the gate off the query is left alone, since any token it carried was refused while the
// query was prepared, and no token is minted. Whether the reader can paginate plays no part: a
// reader declaring Paginated false returns no cursor (SearchCapabilities.Paginated), and a cursor
// it returns anyway is wrapped like any other and refused on the next request (RFC 0014 §6.2).
func resumeSpanSearch(query *tracestore.SpanQueryParams) (pageTokenMinter, error) {
	if !PaginationGate.IsEnabled() {
		return pageTokenMinter{}, nil
	}
	minted, err := pagetoken.FromSpanQuery(*query)
	if err != nil {
		return pageTokenMinter{}, err
	}
	minter := pageTokenMinter{template: minted}
	query.Pagination.PageToken, err = minter.resume(query.Pagination.PageToken)
	return minter, err
}

// resumeTraceSearch is resumeSpanSearch for a trace search, which paginates whenever Pagination is
// present. Pagination is the copy prepareAndInterceptSearchQuery clamped, so the exchange does not
// rewrite the caller's request.
func resumeTraceSearch(query *tracestore.TraceQueryParams) (pageTokenMinter, error) {
	if query.Pagination == nil {
		return pageTokenMinter{}, nil
	}
	minted, err := pagetoken.FromTraceQuery(*query)
	if err != nil {
		return pageTokenMinter{}, err
	}
	minter := pageTokenMinter{template: minted}
	query.Pagination.PageToken, err = minter.resume(query.Pagination.PageToken)
	return minter, err
}

// resume exchanges the page token a client sent for the reader's cursor it wraps. The token is
// refused when it continues a different query than the one it arrived with, since its cursor is a
// position in that query's ordering alone. An empty token starts a new search and passes through
// unchanged.
func (m pageTokenMinter) resume(token []byte) ([]byte, error) {
	if len(token) == 0 {
		return nil, nil
	}
	received, err := pagetoken.DecodeString(string(token))
	if err != nil {
		return nil, err
	}
	if err := received.Verify(m.template.Fingerprint); err != nil {
		return nil, err
	}
	return received.Cursor, nil
}

// mint turns the cursor a reader returned on a page's final chunk into the token the client
// receives. An empty cursor means the last page and stays empty, and a search that paginates
// nothing is answered with no token whatever the reader returned (RFC 0014 §4, §6.2).
func (m pageTokenMinter) mint(cursor []byte) (string, error) {
	if m.template == nil || len(cursor) == 0 {
		return "", nil
	}
	return pagetoken.EncodeToString(&pagetoken.PageToken{
		Version:     m.template.Version,
		Fingerprint: m.template.Fingerprint,
		Cursor:      cursor,
	})
}
