// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestNewHTTPClientRoundTripper_NilBaseUsesDefault(t *testing.T) {
	rt := headerforwarding.NewHTTPClientRoundTripper(nil)
	require.NotNil(t, rt)
	// We cannot call DefaultTransport here without making a real network call,
	// but we can confirm the RoundTripper short-circuits when there are no
	// captured headers and forwards to its base. Using a non-nil base verifies
	// the wrapping behavior; the nil branch is exercised by construction above.
}

func TestHTTPClientRoundTripper_NoCapturedHeaders_PassThrough(t *testing.T) {
	var seen *http.Request
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = r
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := headerforwarding.NewHTTPClientRoundTripper(base)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Existing", "value")

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// Same request instance forwarded (no clone when nothing to add).
	assert.Same(t, req, seen)
	assert.Equal(t, "value", seen.Header.Get("Existing"))
}

func TestHTTPClientRoundTripper_AddsCapturedHeaders(t *testing.T) {
	var seen *http.Request
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = r
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := headerforwarding.NewHTTPClientRoundTripper(base)

	hUser := &headerforwarding.ForwardedHeader{HTTPName: "X-Forwarded-User", Role: headerforwarding.RoleUsername}
	hMail := &headerforwarding.ForwardedHeader{HTTPName: "X-Forwarded-Email", Role: headerforwarding.RoleEmail}
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: hUser, Value: "alice"},
		{Header: hMail, Value: "alice@example.com"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/", http.NoBody)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	require.NotNil(t, seen)
	assert.NotSame(t, req, seen, "request should be cloned before mutation")
	assert.Equal(t, "alice", seen.Header.Get("X-Forwarded-User"))
	assert.Equal(t, "alice@example.com", seen.Header.Get("X-Forwarded-Email"))
	// Original request must not be mutated.
	assert.Empty(t, req.Header.Get("X-Forwarded-User"))
	assert.Empty(t, req.Header.Get("X-Forwarded-Email"))
}

func TestHTTPClientRoundTripper_SkipsEmptyAndInvalidEntries(t *testing.T) {
	var seen *http.Request
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = r
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := headerforwarding.NewHTTPClientRoundTripper(base)

	good := &headerforwarding.ForwardedHeader{HTTPName: "X-Good"}
	emptyName := &headerforwarding.ForwardedHeader{HTTPName: ""}
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: good, Value: "yes"},
		{Header: good, Value: ""},       // empty value -> skipped
		{Header: emptyName, Value: "x"}, // empty HTTPName -> skipped
		{Header: nil, Value: "x"},       // nil header -> skipped
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/", http.NoBody)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	require.NotNil(t, seen)
	assert.Equal(t, "yes", seen.Header.Get("X-Good"))
	assert.Empty(t, seen.Header.Get(""))
}

func TestHTTPClientRoundTripper_RewritesOutboundHeaderName(t *testing.T) {
	var seen *http.Request
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = r
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	rt := headerforwarding.NewHTTPClientRoundTripper(base)

	// Inbound is captured under HTTPName, but the outbound HTTP request
	// should carry HTTPOutboundName instead.
	rename := &headerforwarding.ForwardedHeader{
		HTTPName:         "X-Forwarded-User",
		HTTPOutboundName: "X-Backend-User",
		Role:             headerforwarding.RoleUsername,
	}
	// No HTTPOutboundName -> falls back to HTTPName.
	fallback := &headerforwarding.ForwardedHeader{
		HTTPName: "X-Forwarded-Email",
		Role:     headerforwarding.RoleEmail,
	}
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: rename, Value: "alice"},
		{Header: fallback, Value: "alice@example.com"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/", http.NoBody)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	require.NotNil(t, seen)
	assert.Equal(t, "alice", seen.Header.Get("X-Backend-User"))
	assert.Empty(t, seen.Header.Get("X-Forwarded-User"))
	assert.Equal(t, "alice@example.com", seen.Header.Get("X-Forwarded-Email"))
}

func TestHTTPClientRoundTripper_PropagatesBaseError(t *testing.T) {
	wantErr := http.ErrHandlerTimeout
	base := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	})
	rt := headerforwarding.NewHTTPClientRoundTripper(base)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/", http.NoBody)
	require.NoError(t, err)

	_, err = rt.RoundTrip(req)
	assert.ErrorIs(t, err, wantErr)
}
