// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/auth/apikey"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
)

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}

func TestRoundTripper(t *testing.T) {
	for _, tc := range []struct {
		name             string
		tokenFn          func() string
		authScheme       string
		wrappedTransport http.RoundTripper
		requestContext   context.Context
		wantHeader       string
		wantError        bool
		FromCtxFn        func(context.Context) (string, bool)
	}{
		{
			name:       "No token function, no context: header empty",
			tokenFn:    nil,
			authScheme: "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Empty(t, r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "",
		},
		{
			name:       "TokenFn returns token, no context: header set",
			tokenFn:    func() string { return "mytoken" },
			authScheme: "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer mytoken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "Bearer mytoken",
		},
		{
			name:       "Override from context, context token present: context wins",
			tokenFn:    func() string { return "filetoken" },
			authScheme: "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer ctxToken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: bearertoken.ContextWithBearerToken(context.Background(), "ctxToken"),
			wantHeader:     "Bearer ctxToken",
			FromCtxFn:      bearertoken.GetBearerToken,
		},
		{
			name:       "Context check enabled but context empty: file token used",
			tokenFn:    func() string { return "filetoken" },
			authScheme: "Bearer",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer filetoken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "Bearer filetoken",
		},
		{
			name:       "ApiKey: TokenFn only, no FromCtxFn",
			tokenFn:    func() string { return "apiKeyToken" },
			authScheme: "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey apiKeyToken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: context.Background(),
			wantHeader:     "ApiKey apiKeyToken",
		},
		{
			name:       "ApiKey: context token present and FromCtxFn set: context wins",
			tokenFn:    func() string { return "apiKeyToken" },
			authScheme: "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey contextApiKey", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: apikey.ContextWithAPIKey(context.Background(), "contextApiKey"),
			wantHeader:     "ApiKey contextApiKey",
			FromCtxFn:      apikey.GetAPIKey,
		},
		{
			name:       "ApiKey: context token empty and FromCtxFn set: fallback to TokenFn",
			tokenFn:    func() string { return "apiKeyToken" },
			authScheme: "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey apiKeyToken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: apikey.ContextWithAPIKey(context.Background(), ""),
			wantHeader:     "ApiKey apiKeyToken",
			FromCtxFn:      apikey.GetAPIKey,
		},
		{
			name:       "ApiKey: FromCtxFn nil: use TokenFn",
			tokenFn:    func() string { return "apiKeyToken" },
			authScheme: "ApiKey",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "ApiKey apiKeyToken", r.Header.Get("Authorization"))
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			requestContext: apikey.ContextWithAPIKey(context.Background(), "contextApiKey"),
			wantHeader:     "ApiKey apiKeyToken",
			FromCtxFn:      nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(nil)
			defer server.Close()
			req, err := http.NewRequestWithContext(tc.requestContext, http.MethodGet, server.URL, nil)
			require.NoError(t, err)

			tr := RoundTripper{
				Transport:  tc.wrappedTransport,
				AuthScheme: tc.authScheme,
				TokenFn:    tc.tokenFn,
				FromCtxFn:  tc.FromCtxFn,
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
