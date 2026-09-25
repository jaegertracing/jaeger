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

func TestSealOpenRoundTrip(t *testing.T) {
	want := Token{Storage: "primary", Fingerprint: []byte{1, 2, 3}, Cursor: `[1727280000000000,"trace","span"]`}
	sealed := Seal(want)
	assert.NotContains(t, sealed, "=", "the token is URL-safe without padding")

	got, err := Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestSealOpenEmptyFields(t *testing.T) {
	got, err := Open(Seal(Token{}))
	require.NoError(t, err)
	assert.Empty(t, got.Storage)
	assert.Empty(t, got.Fingerprint)
	assert.Empty(t, got.Cursor)
}

// TestOpenSkipsUnknownFields pins the additive evolution path: a field this version does not
// know is skipped rather than refused, so a field can be added without a version bump.
func TestOpenSkipsUnknownFields(t *testing.T) {
	var b []byte
	b = protowire.AppendTag(b, fieldVersion, protowire.VarintType)
	b = protowire.AppendVarint(b, Version)
	b = protowire.AppendTag(b, 9, protowire.BytesType)
	b = protowire.AppendString(b, "from the future")
	b = protowire.AppendTag(b, 10, protowire.VarintType)
	b = protowire.AppendVarint(b, 42)
	b = protowire.AppendTag(b, fieldCursor, protowire.BytesType)
	b = protowire.AppendString(b, "cursor")

	got, err := Open(base64.RawURLEncoding.EncodeToString(b))
	require.NoError(t, err)
	assert.Equal(t, "cursor", got.Cursor)
}

func TestOpenRefusals(t *testing.T) {
	encode := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	truncated := encode(protowire.AppendTag(nil, fieldCursor, protowire.BytesType))
	var wrongVersion []byte
	wrongVersion = protowire.AppendTag(wrongVersion, fieldVersion, protowire.VarintType)
	wrongVersion = protowire.AppendVarint(wrongVersion, Version+1)
	var badTag []byte
	badTag = protowire.AppendVarint(badTag, uint64(protowire.EncodeTag(fieldStorage, 7)))

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
			_, err := Open(tc.token)
			require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestVerify(t *testing.T) {
	token := Token{Storage: "primary", Fingerprint: []byte{1, 2, 3}}

	require.NoError(t, token.Verify("primary", []byte{1, 2, 3}))

	err := token.Verify("archive", []byte{1, 2, 3})
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	require.ErrorContains(t, err, "different storage")

	err = token.Verify("primary", []byte{4, 5, 6})
	require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
	require.ErrorContains(t, err, "different query")

	err = Token{Storage: "archive", Fingerprint: []byte{4}}.Verify("primary", []byte{1})
	require.ErrorContains(t, err, "different storage", "the storage is checked before the query")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
