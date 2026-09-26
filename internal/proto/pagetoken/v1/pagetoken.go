// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package pagetoken is the page token a paginating storage Reader returns with a page and
// reads back on continuation (RFC 0014 §3). The PageToken message is generated from
// page_token.proto beside this file; this file adds the string form a client sees and the
// fingerprint that binds a token to the query it was returned for.
//
// The token wraps the Reader's cursor with the facts that make it safe to honor later: the
// format version and a fingerprint of the query the cursor is a position in. The query service
// passes the token through unchanged, so a Reader that declares SearchCapabilities.Paginated
// builds its token here and, on continuation, compares the fingerprint DecodeFingerprint
// returns with that of the query it receives; the comparison is the Reader's. The token is not
// signed, so a Reader still has to refuse a cursor it cannot interpret: the token guards
// against a client's mistake, not against a client's intent.
package pagetoken

import (
	"encoding/base64"
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Version is the token format this package writes and the only one it reads. It changes when a
// token minted under the previous format would still decode but mean something else, such as
// after the sort order behind a cursor changes. An added field does not need it, because the
// generated Unmarshal skips fields it does not know.
const Version = 1

// EncodeFingerprint returns the token a client receives for a reader's cursor and echoes back
// on the next request: a PageToken with the current Version, the fingerprint of the query, and the
// cursor, as its protobuf encoding in URL-safe base64 without padding.
func EncodeFingerprint(fingerprint, cursor []byte) (string, error) {
	t := &PageToken{Version: Version, Fingerprint: fingerprint, Cursor: cursor}
	raw, err := t.Marshal()
	if err != nil {
		return "", fmt.Errorf("page token cannot be encoded: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeFingerprint returns the fingerprint and the cursor a token the client sent back carries.
// It checks only what the token says about itself, the version and that it carries a cursor;
// whether the fingerprint is that of the request it arrived with is the caller's to compare. A
// token without a cursor is refused because a Reader never returns one: the last page carries
// an empty token, not a token around an empty cursor, so accepting it would restart the search
// while the client believes it is continuing.
func DecodeFingerprint(token string) (fingerprint, cursor []byte, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: page token is not valid base64", tracestore.ErrPaginationInvalid)
	}
	t := &PageToken{}
	if err := t.Unmarshal(raw); err != nil {
		return nil, nil, fmt.Errorf("%w: page token is malformed: %w", tracestore.ErrPaginationInvalid, err)
	}
	if t.Version != Version {
		return nil, nil, fmt.Errorf("%w: page token version %d is not supported", tracestore.ErrPaginationInvalid, t.Version)
	}
	if len(t.Cursor) == 0 {
		return nil, nil, fmt.Errorf("%w: page token carries no cursor", tracestore.ErrPaginationInvalid)
	}
	return t.Fingerprint, t.Cursor, nil
}
