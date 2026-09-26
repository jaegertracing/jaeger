// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/jaegertracing/jaeger/internal/proto/pagetoken/v1"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	fingerprint, cursor := []byte{1, 2, 3}, []byte(`[1727280000000000,"trace","span"]`)
	encoded, err := NewPageToken(fingerprint, cursor)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "=", "the token is URL-safe without padding")

	gotFingerprint, gotCursor, err := encoded.Decode()
	require.NoError(t, err)
	assert.Equal(t, fingerprint, gotFingerprint)
	assert.Equal(t, cursor, gotCursor)
}

// TestDecodeRefusals covers what Decode adds over the generated Unmarshal: the base64
// framing, the version check, and every failure reading as a bad request.
// TestDecode_SkipsAnUnknownField pins the forward compatibility PageTokenVersion relies on: a field
// added to the message later is skipped by a decoder that predates it, so adding one needs no
// version bump (RFC 0014 §3.1).
func TestDecode_SkipsAnUnknownField(t *testing.T) {
	raw, err := (&pb.PageToken{Version: PageTokenVersion, Fingerprint: []byte{1}, Cursor: []byte("c")}).Marshal()
	require.NoError(t, err)
	// Field 4, wire type 0 (varint), value 7: a field this version of the message does not have.
	raw = append(raw, 0x20, 0x07)

	fingerprint, cursor, err := PageToken(base64.RawURLEncoding.EncodeToString(raw)).Decode()
	require.NoError(t, err)
	assert.Equal(t, []byte{1}, fingerprint)
	assert.Equal(t, []byte("c"), cursor)
}

// TestCursor pins the one check Cursor adds over Decode: the token has to belong to the query
// it is resumed for.
func TestCursor(t *testing.T) {
	token, err := NewPageToken([]byte{1, 2, 3}, []byte("cursor"))
	require.NoError(t, err)

	cursor, err := token.Cursor([]byte{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, []byte("cursor"), cursor)

	_, err = token.Cursor([]byte{4, 5, 6})
	require.ErrorIs(t, err, ErrPaginationInvalid)
	assert.ErrorContains(t, err, "different query")

	_, err = PageToken("not-a-token!").Cursor([]byte{1, 2, 3})
	require.ErrorIs(t, err, ErrPaginationInvalid, "a token that does not decode is refused before the comparison")
}

func TestDecodeRefusals(t *testing.T) {
	encoded := func(token *pb.PageToken) string {
		raw, err := token.Marshal()
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{name: "not base64", token: "not-a-token!", want: "not valid base64"},
		{name: "not a message", token: base64.RawURLEncoding.EncodeToString([]byte{0x1a}), want: "malformed"},
		{name: "no version", token: encoded(&pb.PageToken{}), want: "version 0 is not supported"},
		{name: "future version", token: encoded(&pb.PageToken{Version: PageTokenVersion + 1}), want: "version 2 is not supported"},
		{name: "no cursor", token: encoded(&pb.PageToken{Version: PageTokenVersion, Fingerprint: []byte{1}}), want: "carries no cursor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := PageToken(tc.token).Decode()
			require.ErrorIs(t, err, ErrPaginationInvalid)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
