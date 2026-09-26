// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"bytes"
	"encoding/base64"
	"fmt"

	pb "github.com/jaegertracing/jaeger/internal/proto/pagetoken/v1"
)

// PageToken is a Reader's own opaque continuation token (RFC 0014 §3). The type names whose
// value it is, not a format: the query service and the jaeger.storage.v2 wire carry it to the
// client and back unchanged, so a Reader receives on continuation exactly the token it returned
// in PageChunk.NextPageToken, and only that Reader reads it. A backend whose storage validates a
// continuation token of its own returns that token in this type as it is. A Reader MUST refuse a
// token it did not produce, or produced for a different query, with ErrPaginationInvalid rather
// than send its contents to the backend.
//
// For a Reader that has no token of its own, this package provides one. NewPageToken wraps the
// Reader's cursor with the format version and the Fingerprint of the query it received, and
// Cursor is the only way to read the cursor back: it refuses a token that does not decode or
// that belongs to another query. The token is not signed, so a Reader still has to refuse a
// cursor it cannot interpret: the token guards against a client's mistake, not against a
// client's intent. The wire form is the PageToken message of internal/proto/pagetoken/v1.
type PageToken string

// PageTokenVersion is the token format NewPageToken writes and the only one Cursor reads. It
// changes when a token written under the previous format would still decode but mean something
// else, such as after the sort order behind a cursor changes. An added field does not need it,
// because the generated Unmarshal skips fields it does not know.
const PageTokenVersion = 1

// NewPageToken wraps a Reader's cursor with the current PageTokenVersion and the fingerprint of
// the query, as the protobuf encoding of the token message in URL-safe base64 without padding.
func NewPageToken(fingerprint, cursor []byte) (PageToken, error) {
	t := &pb.PageToken{Version: PageTokenVersion, Fingerprint: fingerprint, Cursor: cursor}
	raw, err := t.Marshal()
	if err != nil {
		return "", fmt.Errorf("page token cannot be encoded: %w", err)
	}
	return PageToken(base64.RawURLEncoding.EncodeToString(raw)), nil
}

// decode returns the fingerprint and the cursor the token carries. It checks only what the token
// says about itself, the version and that it carries a cursor; whether the fingerprint is that
// of the query the token arrived with is Cursor's check. A token without a cursor is refused
// because a Reader never returns one: the last page carries an empty token, not a token around
// an empty cursor, so accepting it would restart the search while the client believes it is
// continuing.
func (t PageToken) decode() (fingerprint, cursor []byte, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(string(t))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: page token is not valid base64", ErrPaginationInvalid)
	}
	m := &pb.PageToken{}
	if err := m.Unmarshal(raw); err != nil {
		return nil, nil, fmt.Errorf("%w: page token is malformed: %w", ErrPaginationInvalid, err)
	}
	if m.Version != PageTokenVersion {
		return nil, nil, fmt.Errorf("%w: page token version %d is not supported", ErrPaginationInvalid, m.Version)
	}
	if len(m.Cursor) == 0 {
		return nil, nil, fmt.Errorf("%w: page token carries no cursor", ErrPaginationInvalid)
	}
	return m.Fingerprint, m.Cursor, nil
}

// Cursor returns the cursor the token wraps, for a Reader resuming the query with the given
// fingerprint. A token returned for a different query is refused, since its cursor is a position
// in that query's ordering alone (RFC 0014 §3.2).
func (t PageToken) Cursor(fingerprint []byte) ([]byte, error) {
	got, cursor, err := t.decode()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(got, fingerprint) {
		return nil, fmt.Errorf("%w: page token belongs to a different query; continue with the query it was returned for",
			ErrPaginationInvalid)
	}
	return cursor, nil
}
