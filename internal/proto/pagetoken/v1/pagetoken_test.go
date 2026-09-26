// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package pagetoken

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

// Field numbers of page_token.proto, for building tokens by hand.
const (
	fieldVersion protowire.Number = 1
	fieldCursor  protowire.Number = 3
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	want := &PageToken{Version: Version, Fingerprint: []byte{1, 2, 3}, Cursor: []byte(`[1727280000000000,"trace","span"]`)}
	encoded, err := EncodeToString(want)
	require.NoError(t, err)
	assert.NotContains(t, encoded, "=", "the token is URL-safe without padding")

	got, err := DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, want.Version, got.Version)
	assert.Equal(t, want.Fingerprint, got.Fingerprint)
	assert.Equal(t, want.Cursor, got.Cursor)
}

func TestEncodeDecodeEmptyFields(t *testing.T) {
	encoded, err := EncodeToString(&PageToken{Version: Version})
	require.NoError(t, err)
	got, err := DecodeString(encoded)
	require.NoError(t, err)
	assert.Empty(t, got.Fingerprint)
	assert.Empty(t, got.Cursor)
}

// TestDecodeSkipsUnknownFields pins the additive evolution path: a field this version does not
// know is skipped rather than refused, so a field can be added without a version bump.
func TestDecodeSkipsUnknownFields(t *testing.T) {
	var b []byte
	b = protowire.AppendTag(b, fieldVersion, protowire.VarintType)
	b = protowire.AppendVarint(b, Version)
	b = protowire.AppendTag(b, 9, protowire.BytesType)
	b = protowire.AppendString(b, "from the future")
	b = protowire.AppendTag(b, 10, protowire.VarintType)
	b = protowire.AppendVarint(b, 42)
	b = protowire.AppendTag(b, fieldCursor, protowire.BytesType)
	b = protowire.AppendString(b, "cursor")

	got, err := DecodeString(base64.RawURLEncoding.EncodeToString(b))
	require.NoError(t, err)
	assert.Equal(t, []byte("cursor"), got.Cursor)
}

func TestDecodeRefusals(t *testing.T) {
	encode := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	truncated := encode(protowire.AppendTag(nil, fieldCursor, protowire.BytesType))
	var wrongVersion []byte
	wrongVersion = protowire.AppendTag(wrongVersion, fieldVersion, protowire.VarintType)
	wrongVersion = protowire.AppendVarint(wrongVersion, Version+1)
	var badTag []byte
	badTag = protowire.AppendVarint(badTag, uint64(protowire.EncodeTag(fieldCursor, 7)))

	cases := []struct {
		name  string
		token string
		want  string
	}{
		{name: "not base64", token: "not-a-token!", want: "not valid base64"},
		{name: "invalid tag", token: encode(protowire.AppendVarint(nil, 0)), want: "malformed"},
		{name: "truncated field", token: truncated, want: "malformed"},
		{name: "invalid wire type", token: encode(badTag), want: "malformed"},
		{name: "no version", token: encode(nil), want: "version 0 is not supported"},
		{name: "future version", token: encode(wrongVersion), want: "version 2 is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeString(tc.token)
			require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestVerify(t *testing.T) {
	token := &PageToken{Fingerprint: []byte{1, 2, 3}}

	require.NoError(t, token.Verify([]byte{1, 2, 3}))

	err := token.Verify([]byte{4, 5, 6})
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	require.ErrorContains(t, err, "different query")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
