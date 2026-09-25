// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc/pagetoken"
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

// resumeCursor exchanges the page token a client sent for the reader's cursor it wraps. The
// token is refused when a different storage minted it or when it continues a different query
// than the one it arrived with, since its cursor is a position in that query's ordering alone
// (RFC 0014 §3.2). An empty token starts a new search and passes through unchanged.
func (qs QueryService) resumeCursor(token string, fingerprint []byte) (string, error) {
	if token == "" {
		return "", nil
	}
	opened, err := pagetoken.Open(token)
	if err != nil {
		return "", err
	}
	if err := opened.Verify(qs.options.TraceStorageName, fingerprint); err != nil {
		return "", err
	}
	return opened.Cursor, nil
}

// nextPageToken wraps the cursor a reader returned on a page's final chunk into the token the
// client receives. An empty cursor means the last page and stays empty.
func (qs QueryService) nextPageToken(cursor string, fingerprint []byte) string {
	if cursor == "" {
		return ""
	}
	return pagetoken.Seal(pagetoken.Token{
		Storage:     qs.options.TraceStorageName,
		Fingerprint: fingerprint,
		Cursor:      cursor,
	})
}
