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
	fingerprint, cursor := []byte{1, 2, 3}, []byte(`[1727280000000000,"trace","span"]`)
	encoded, err := EncodeFingerprint(fingerprint, cursor)
	require.NoError(t, err)
	assert.NotContains(t, encoded, "=", "the token is URL-safe without padding")

	gotFingerprint, gotCursor, err := DecodeFingerprint(encoded)
	require.NoError(t, err)
	assert.Equal(t, fingerprint, gotFingerprint)
	assert.Equal(t, cursor, gotCursor)
}

// TestDecodeRefusals covers what DecodeFingerprint adds over the generated Unmarshal: the base64
// framing, the version check, and every failure reading as a bad request.
func TestDecodeRefusals(t *testing.T) {
	encoded := func(token *PageToken) string {
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
		{name: "no version", token: encoded(&PageToken{}), want: "version 0 is not supported"},
		{name: "future version", token: encoded(&PageToken{Version: Version + 1}), want: "version 2 is not supported"},
		{name: "no cursor", token: encoded(&PageToken{Version: Version, Fingerprint: []byte{1}}), want: "carries no cursor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := DecodeFingerprint(tc.token)
			require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
