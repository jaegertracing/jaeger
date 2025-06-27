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
		tokenFn          func() string
		overrideFromCtx  bool
		authScheme       string
		wrappedTransport http.RoundTripper
		requestContext   context.Context
		wantHeader       string
		wantError        bool
	}{
		{
			name:            "No token function, no context: header empty",
			tokenFn:         nil,
			overrideFromCtx: false,
			authScheme:      "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Empty(t, r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "",
		},
		{
			name:            "TokenFn returns token, no context: header set",
			tokenFn:         func() string { return "mytoken" },
			overrideFromCtx: false,
			authScheme:      "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer mytoken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "Bearer mytoken",
		},
		{
			name:            "Override from context, context token present: context wins",
			tokenFn:         func() string { return "filetoken" },
			overrideFromCtx: true,
			authScheme:      "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer ctxToken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: ContextWithBearerToken(context.Background(), "ctxToken"),
			wantHeader:     "Bearer ctxToken",
		},
		{
			name:            "Override from context enabled but context empty: file token used",
			tokenFn:         func() string { return "filetoken" },
			overrideFromCtx: true,
			authScheme:      "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer filetoken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "Bearer filetoken",
		},
		{
			name:            "ApiKey: TokenFn returns token, no context: header set",
			tokenFn:         func() string { return "apiKeyToken" },
			overrideFromCtx: false,
			authScheme:      "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey apiKeyToken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "ApiKey apiKeyToken",
		},
		{
			name:            "ApiKey: Override from context, context token present: context wins",
			tokenFn:         func() string { return "apiKeyToken" },
			overrideFromCtx: true,
			authScheme:      "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey contextApiKey", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: ContextWithAPIKey(context.Background(), "contextApiKey"),
			wantHeader:     "ApiKey contextApiKey",
		},
		{
			name:           "Nil roundTripper provided should return an error",
			requestContext: context.Background(),
			wantError:      true,
		},
		{
			name:            "ApiKey: TokenFn returns token, no context: header set",
			tokenFn:         func() string { return "apiKeyToken" },
			overrideFromCtx: false,
			authScheme:      "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey apiKeyToken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "ApiKey apiKeyToken",
		},
		{
			name:            "ApiKey: Override from context, context token present: context wins",
			tokenFn:         func() string { return "apiKeyToken" },
			overrideFromCtx: true,
			authScheme:      "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey contextApiKey", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: ContextWithAPIKey(context.Background(), "contextApiKey"),
			wantHeader:     "ApiKey contextApiKey",
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
				AuthScheme:      tc.authScheme,
				TokenFn:         tc.tokenFn,
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
