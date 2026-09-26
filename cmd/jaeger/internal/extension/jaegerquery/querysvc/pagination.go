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

// pageTokens binds the page tokens of one search to the query as dispatched (RFC 0014 §3.2): the
// token a client sent is exchanged for the reader's cursor it wraps, and the cursor the reader
// returns is wrapped into the token the client receives. The zero value belongs to a search that
// paginates nothing, which mints no token; any token such a search carried was refused while the
// query was prepared.
type pageTokens struct {
	// minted is the token every page of this search is minted from, with the cursor left empty;
	// nil for a search that paginates nothing.
	minted *pagetoken.PageToken
}

// resumeSpanSearch exchanges the page token in query for the reader's cursor, in place, and returns
// the pageTokens that wrap the reader's next cursor. paginates says whether the search mints a
// token at all; where it does not, the query is left alone.
func resumeSpanSearch(query *tracestore.SpanQueryParams, paginates bool) (pageTokens, error) {
	if !paginates {
		return pageTokens{}, nil
	}
	minted, err := pagetoken.FromSpanQuery(*query)
	if err != nil {
		return pageTokens{}, err
	}
	tokens := pageTokens{minted: minted}
	query.Pagination.PageToken, err = tokens.resume(query.Pagination.PageToken)
	return tokens, err
}

// resumeTraceSearch is resumeSpanSearch for a trace search, which paginates whenever Pagination is
// present. The exchange lands on a copy of Pagination so the caller's request is not rewritten
// through the shared pointer.
func resumeTraceSearch(query *tracestore.TraceQueryParams) (pageTokens, error) {
	if query.Pagination == nil {
		return pageTokens{}, nil
	}
	minted, err := pagetoken.FromTraceQuery(*query)
	if err != nil {
		return pageTokens{}, err
	}
	tokens := pageTokens{minted: minted}
	resumed := *query.Pagination
	resumed.PageToken, err = tokens.resume(resumed.PageToken)
	query.Pagination = &resumed
	return tokens, err
}

// resume exchanges the page token a client sent for the reader's cursor it wraps. The token is
// refused when it continues a different query than the one it arrived with, since its cursor is a
// position in that query's ordering alone. An empty token starts a new search and passes through
// unchanged.
func (p pageTokens) resume(token string) (string, error) {
	if token == "" {
		return "", nil
	}
	received, err := pagetoken.DecodeString(token)
	if err != nil {
		return "", err
	}
	if err := received.Verify(p.minted.Fingerprint); err != nil {
		return "", err
	}
	return string(received.Cursor), nil
}

// wrap turns the cursor a reader returned on a page's final chunk into the token the client
// receives. An empty cursor means the last page and stays empty, and a search that paginates
// nothing is answered with no token whatever the reader returned (RFC 0014 §4, §6.2).
func (p pageTokens) wrap(cursor string) (string, error) {
	if p.minted == nil || cursor == "" {
		return "", nil
	}
	return pagetoken.EncodeToString(&pagetoken.PageToken{
		Version:     p.minted.Version,
		Fingerprint: p.minted.Fingerprint,
		Cursor:      []byte(cursor),
	})
}
