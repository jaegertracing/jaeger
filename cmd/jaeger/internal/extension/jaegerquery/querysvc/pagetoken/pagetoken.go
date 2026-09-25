// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package pagetoken encodes the page token the query service hands to a client and reads back
// on continuation (RFC 0014 §3). The token wraps the cursor a storage reader returned with the
// facts that make it safe to honor later: the format version, the name of the storage whose
// reader minted the cursor, and a fingerprint of the query the cursor is a position in. A reader
// only ever sees its own cursor; the wrapping and the checks belong to the query service.
package pagetoken

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Version is the token format this package writes and the only one it reads. It changes when a
// token minted under the previous format would still decode but mean something else, such as
// after the sort order behind a cursor changes. An added field does not need it, because Open
// skips fields it does not know.
const Version = 1

// The token is a protobuf wire message, encoded by hand because it has four fields and no other
// component ever decodes it, so a generated message would buy nothing for its .proto file.
const (
	fieldVersion     protowire.Number = 1
	fieldStorage     protowire.Number = 2
	fieldFingerprint protowire.Number = 3
	fieldCursor      protowire.Number = 4
)

// Token is what a page token carries besides its version.
type Token struct {
	// Storage names the trace storage whose reader minted Cursor, as the deployment configured
	// it, so that a token presented to a different storage is refused (RFC 0014 §3.2).
	Storage string
	// Fingerprint identifies the query the cursor is a position in; see TraceQuery and SpanQuery.
	Fingerprint []byte
	// Cursor is the continuation cursor as the reader returned it in PageChunk.NextPageToken.
	// It is opaque here: the reader that minted it is the only one that interprets it.
	Cursor string
}

// Seal encodes the token as the string the client receives and echoes back.
func Seal(t Token) string {
	var b []byte
	b = protowire.AppendTag(b, fieldVersion, protowire.VarintType)
	b = protowire.AppendVarint(b, Version)
	b = protowire.AppendTag(b, fieldStorage, protowire.BytesType)
	b = protowire.AppendString(b, t.Storage)
	b = protowire.AppendTag(b, fieldFingerprint, protowire.BytesType)
	b = protowire.AppendBytes(b, t.Fingerprint)
	b = protowire.AppendTag(b, fieldCursor, protowire.BytesType)
	b = protowire.AppendString(b, t.Cursor)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Open decodes a token the client sent back. It checks only what the token says about itself;
// whether it belongs to the request it arrived with is Token.Verify.
func Open(token string) (Token, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Token{}, fmt.Errorf("%w: page token is not valid base64", tracestore.ErrPaginationInvalid)
	}
	var t Token
	var version uint64
	for len(raw) > 0 {
		num, typ, n := protowire.ConsumeTag(raw)
		if n < 0 {
			return Token{}, malformed(n)
		}
		raw = raw[n:]
		switch {
		case num == fieldVersion && typ == protowire.VarintType:
			version, n = protowire.ConsumeVarint(raw)
		case num == fieldStorage && typ == protowire.BytesType:
			t.Storage, n = protowire.ConsumeString(raw)
		case num == fieldFingerprint && typ == protowire.BytesType:
			t.Fingerprint, n = protowire.ConsumeBytes(raw)
		case num == fieldCursor && typ == protowire.BytesType:
			t.Cursor, n = protowire.ConsumeString(raw)
		default:
			n = protowire.ConsumeFieldValue(num, typ, raw)
		}
		if n < 0 {
			return Token{}, malformed(n)
		}
		raw = raw[n:]
	}
	if version != Version {
		return Token{}, fmt.Errorf("%w: page token version %d is not supported", tracestore.ErrPaginationInvalid, version)
	}
	return t, nil
}

func malformed(n int) error {
	return fmt.Errorf("%w: page token is malformed: %w", tracestore.ErrPaginationInvalid, protowire.ParseError(n))
}

// Verify refuses a token that cannot resume a search on the named storage with the given query
// fingerprint. The storage is checked first, because a token from another storage says nothing
// about this one's queries.
func (t Token) Verify(storage string, fingerprint []byte) error {
	if t.Storage != storage {
		return fmt.Errorf("%w: page token was issued by a different storage", tracestore.ErrPaginationInvalid)
	}
	if !bytes.Equal(t.Fingerprint, fingerprint) {
		return fmt.Errorf("%w: page token belongs to a different query; continue with the query it was returned for",
			tracestore.ErrPaginationInvalid)
	}
	return nil
}
