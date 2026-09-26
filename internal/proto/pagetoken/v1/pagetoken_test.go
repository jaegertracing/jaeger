// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package pagetoken

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/testutils"
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

// TestDecodeRefusals covers what DecodeString adds over the generated Unmarshal: the base64
// framing, the version check, and every failure reading as a bad request.
func TestDecodeRefusals(t *testing.T) {
	encoded := func(token *PageToken) string {
		s, err := EncodeToString(token)
		require.NoError(t, err)
		return s
	}
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{name: "not base64", token: "not-a-token!", want: "not valid base64"},
		{name: "not a message", token: base64.RawURLEncoding.EncodeToString([]byte{0x1a}), want: "malformed"},
		{name: "no version", token: encoded(&PageToken{}), want: "version 0 is not supported"},
		{name: "future version", token: encoded(&PageToken{Version: Version + 1}), want: "version 2 is not supported"},
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
