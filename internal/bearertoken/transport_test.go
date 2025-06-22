// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}

func TestRoundTripper(t *testing.T) {
	for _, tc := range []struct {
		name             string
		staticToken      string
		overrideFromCtx  bool
		authScheme       string
		wrappedTransport http.RoundTripper
		requestContext   context.Context
		wantError        bool
	}{
		{
			name: "Default RoundTripper and request context set should have empty Bearer token",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Empty(t, r.Header.Get("Authorization"))
				return &http.Response{
					StatusCode: http.StatusOK,
				}, nil
			}),
			requestContext: ContextWithBearerToken(context.Background(), "tokenFromContext"),
		},
		{
			name:            "Bearer: Override from context provided, and request context set should use request context token",
			overrideFromCtx: true,
			authScheme:      "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer tokenFromContext", r.Header.Get("Authorization"))
				return &http.Response{
					StatusCode: http.StatusOK,
				}, nil
			}),
			requestContext: ContextWithBearerToken(context.Background(), "tokenFromContext"),
		},
		{
			name:            "Bearer: Allow override from context and token provided, and request context unset should use defaultToken",
			overrideFromCtx: true,
			staticToken:     "initToken",
			authScheme:      "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer initToken", r.Header.Get("Authorization"))
				return &http.Response{}, nil
			}),
			requestContext: context.Background(),
		},
		{
			name:            "Bearer: Allow override from context and token provided, and request context set should use context token",
			overrideFromCtx: true,
			staticToken:     "initToken",
			authScheme:      "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer tokenFromContext", r.Header.Get("Authorization"))
				return &http.Response{}, nil
			}),
			requestContext: ContextWithBearerToken(context.Background(), "tokenFromContext"),
		},
		{
			name:        "ApiKey: Static token should be used with ApiKey scheme",
			staticToken: "apiKeyToken",
			authScheme:  "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey apiKeyToken", r.Header.Get("Authorization"))
				return &http.Response{}, nil
			}),
			requestContext: context.Background(),
		},
		{
			name:            "ApiKey: Override from context should use context API key",
			overrideFromCtx: true,
			staticToken:     "apiKeyToken",
			authScheme:      "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey contextApiKey", r.Header.Get("Authorization"))
				return &http.Response{}, nil
			}),
			requestContext: ContextWithAPIKey(context.Background(), "contextApiKey"),
		},

		{
			name:            "ApiKey: Custom token retrieval function should be used (not supported, should fallback to staticToken)",
			overrideFromCtx: true,
			staticToken:     "apiKeyToken",
			authScheme:      "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey apiKeyToken", r.Header.Get("Authorization"))
				return &http.Response{}, nil
			}),
			requestContext: context.Background(),
		},
		{
			name:           "Nil roundTripper provided should return an error",
			requestContext: context.Background(),
			wantError:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(nil)
			defer server.Close()
			req, err := http.NewRequestWithContext(tc.requestContext, http.MethodGet, server.URL, nil)
			require.NoError(t, err)

			tr := RoundTripper{
				Transport:       tc.wrappedTransport,
				OverrideFromCtx: tc.overrideFromCtx,
				StaticToken:     tc.staticToken,
				AuthScheme:      tc.authScheme,
			}
			resp, err := tr.RoundTrip(req)

			if tc.wantError {
				assert.Nil(t, resp)
				require.Error(t, err)
			} else {
				assert.NotNil(t, resp)
				require.NoError(t, err)
			}
		})
	}
}
