// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc/pagetoken"
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
// returns is sealed into the token the client receives. The zero value belongs to a search that
// paginates nothing, which seals no token; any token such a search carried was refused while the
// query was prepared.
type pageTokens struct {
	fingerprint []byte
	paginates   bool
}

// resumeSpanSearch exchanges the page token in query for the reader's cursor, in place, and returns
// the pageTokens that seal the reader's next cursor. paginates says whether the search mints a
// token at all; where it does not, the query is left alone.
func resumeSpanSearch(query *tracestore.SpanQueryParams, paginates bool) (pageTokens, error) {
	if !paginates {
		return pageTokens{}, nil
	}
	fingerprint, err := pagetoken.SpanQuery(*query)
	if err != nil {
		return pageTokens{}, err
	}
	tokens := pageTokens{fingerprint: fingerprint, paginates: true}
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
	fingerprint, err := pagetoken.TraceQuery(*query)
	if err != nil {
		return pageTokens{}, err
	}
	tokens := pageTokens{fingerprint: fingerprint, paginates: true}
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
	opened, err := pagetoken.Open(token)
	if err != nil {
		return "", err
	}
	if err := opened.Verify(p.fingerprint); err != nil {
		return "", err
	}
	return opened.Cursor, nil
}

// seal wraps the cursor a reader returned on a page's final chunk into the token the client
// receives. An empty cursor means the last page and stays empty, and a search that paginates
// nothing is answered with no token whatever the reader returned (RFC 0014 §4, §6.2).
func (p pageTokens) seal(cursor string) string {
	if !p.paginates || cursor == "" {
		return ""
	}
	return pagetoken.Seal(pagetoken.Token{Fingerprint: p.fingerprint, Cursor: cursor})
}
