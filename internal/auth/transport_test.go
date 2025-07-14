// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net/http"
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
		name           string
		auths          []Config
		requestContext context.Context
		wantHeaders    []string
	}{
		{
			name:           "No auth methods",
			auths:          []Config{},
			requestContext: context.Background(),
			wantHeaders:    nil,
		},
		{
			name: "Single auth: Bearer token from function",
			auths: []Config{
				{
					Scheme:  "Bearer",
					TokenFn: func() string { return "mytoken" },
				},
			},
			requestContext: context.Background(),
			wantHeaders:    []string{"Bearer mytoken"},
		},
		{
			name: "Single auth: Bearer token from context",
			auths: []Config{
				{
					Scheme:  "Bearer",
					FromCtx: bearertoken.GetBearerToken,
				},
			},
			requestContext: bearertoken.ContextWithBearerToken(context.Background(), "ctxToken"),
			wantHeaders:    []string{"Bearer ctxToken"},
		},
		{
			name: "Single auth: API Key from function",
			auths: []Config{
				{
					Scheme:  "ApiKey",
					TokenFn: func() string { return "apiKeyToken" },
				},
			},
			requestContext: context.Background(),
			wantHeaders:    []string{"ApiKey apiKeyToken"},
		},
		{
			name: "Multiple auth methods: API Key and Bearer",
			auths: []Config{
				{
					Scheme:  "ApiKey",
					TokenFn: func() string { return "apiKeyToken" },
				},
				{
					Scheme:  "Bearer",
					FromCtx: bearertoken.GetBearerToken,
				},
			},
			requestContext: bearertoken.ContextWithBearerToken(context.Background(), "bearerToken"),
			wantHeaders:    []string{"ApiKey apiKeyToken", "Bearer bearerToken"},
		},
		{
			name: "Context overrides function token",
			auths: []Config{
				{
					Scheme:  "ApiKey",
					TokenFn: func() string { return "fileToken" },
					FromCtx: apikey.GetAPIKey,
				},
			},
			requestContext: apikey.ContextWithAPIKey(context.Background(), "contextToken"),
			wantHeaders:    []string{"ApiKey contextToken"},
		},
		{
			name: "Multiple auth methods with context",
			auths: []Config{
				{
					Scheme:  "ApiKey",
					TokenFn: func() string { return "apiKeyFromFile" },
					FromCtx: apikey.GetAPIKey,
				},
				{
					Scheme:  "Bearer",
					TokenFn: func() string { return "bearerFromFile" },
					FromCtx: bearertoken.GetBearerToken,
				},
			},
			requestContext: func() context.Context {
				ctx := context.Background()
				ctx = apikey.ContextWithAPIKey(ctx, "apiKeyFromCtx")
				ctx = bearertoken.ContextWithBearerToken(ctx, "bearerFromCtx")
				return ctx
			}(),
			wantHeaders: []string{"ApiKey apiKeyFromCtx", "Bearer bearerFromCtx"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(tc.requestContext, http.MethodGet, "http://test", nil)
			require.NoError(t, err)

			var gotHeaders []string
			mockTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				gotHeaders = r.Header.Values("Authorization")
				return &http.Response{StatusCode: http.StatusOK}, nil
			})

			tr := RoundTripper{
				Transport: mockTransport,
				Auths:     tc.auths,
			}

			resp, err := tr.RoundTrip(req)
			assert.NotNil(t, resp)
			require.NoError(t, err)

			assert.Equal(t, tc.wantHeaders, gotHeaders,
				"Expected Authorization headers: %v, got: %v", tc.wantHeaders, gotHeaders)
		})
	}
}
