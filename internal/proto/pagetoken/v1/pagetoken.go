// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package pagetoken is the page token the query service hands to a client and reads back on
// continuation (RFC 0014 §3). The PageToken message is generated from page_token.proto beside
// this file; this file adds what a token carries besides its fields: the string form a client
// sees, the fingerprint that binds a token to its query, and the check of that binding.
//
// The token wraps the cursor a storage reader returned with the facts that make it safe to
// honor later: the format version and a fingerprint of the query the cursor is a position in.
// A reader only ever sees its own cursor; the wrapping and the checks belong to the query
// service. The token is not signed, so a reader still has to refuse a cursor it cannot
// interpret: the token guards against a client's mistake, not against a client's intent.
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
