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
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Version is the token format this package writes and the only one it reads. It changes when a
// token minted under the previous format would still decode but mean something else, such as
// after the sort order behind a cursor changes. An added field does not need it, because the
// generated Unmarshal skips fields it does not know.
const Version = 1

// EncodeToString encodes the token as the string the client receives and echoes back: the
// protobuf encoding in URL-safe base64 without padding.
func EncodeToString(t *PageToken) (string, error) {
	raw, err := t.Marshal()
	if err != nil {
		return "", fmt.Errorf("page token cannot be encoded: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeString decodes a token the client sent back. It checks only what the token says about
// itself; whether it belongs to the request it arrived with is Verify.
func DecodeString(token string) (*PageToken, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: page token is not valid base64", tracestore.ErrPaginationInvalid)
	}
	t := &PageToken{}
	if err := t.Unmarshal(raw); err != nil {
		return nil, fmt.Errorf("%w: page token is malformed: %w", tracestore.ErrPaginationInvalid, err)
	}
	if t.Version != Version {
		return nil, fmt.Errorf("%w: page token version %d is not supported", tracestore.ErrPaginationInvalid, t.Version)
	}
	return t, nil
}

// Verify refuses a token that continues a different query than the one with the given
// fingerprint.
func (m *PageToken) Verify(fingerprint []byte) error {
	if !bytes.Equal(m.Fingerprint, fingerprint) {
		return fmt.Errorf("%w: page token belongs to a different query; continue with the query it was returned for",
			tracestore.ErrPaginationInvalid)
	}
	return nil
}
