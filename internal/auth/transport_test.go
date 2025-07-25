// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
)

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}

func TestRoundTripper(t *testing.T) {
	tests := []struct {
		name               string
		auths              []Method
		requestContext     context.Context
		expectedHeaders    []string // Expected Authorization headers
		expectError        bool
		expectNoAuthHeader bool
	}{
		{
			name:               "No auth configs - no headers added",
			auths:              []Method{},
			requestContext:     context.Background(),
			expectNoAuthHeader: true,
		},
		{
			name: "Single Bearer auth with static token",
			auths: []Method{
				{
					Scheme:  "Bearer",
					TokenFn: func() string { return "static-token" },
				},
			},
			requestContext:  context.Background(),
			expectedHeaders: []string{"Bearer static-token"},
		},
		{
			name: "Bearer auth with context override - context token used",
			auths: []Method{
				{
					Scheme:  "Bearer",
					TokenFn: func() string { return "static-token" },
					FromCtx: bearertoken.GetBearerToken,
				},
			},
			requestContext:  bearertoken.ContextWithBearerToken(context.Background(), "context-token"),
			expectedHeaders: []string{"Bearer context-token"},
		},
		{
			name: "Multiple auth methods - both tokens added",
			auths: []Method{
				{
					Scheme:  "Bearer",
					TokenFn: func() string { return "bearer-token" },
				},
				{
					Scheme:  "ApiKey",
					TokenFn: func() string { return "api-key-value" },
				},
			},
			requestContext:  context.Background(),
			expectedHeaders: []string{"Bearer bearer-token", "ApiKey api-key-value"},
		},
		{
			name: "Auth config with empty token - no header added",
			auths: []Method{
				{
					Scheme:  "Bearer",
					TokenFn: func() string { return "" }, // Empty token
				},
				{
					Scheme:  "ApiKey",
					TokenFn: func() string { return "valid-key" },
				},
			},
			requestContext:  context.Background(),
			expectedHeaders: []string{"ApiKey valid-key"}, // Only valid token
		},
		{
			name: "Only context extraction, no context token - no header added",
			auths: []Method{
				{
					Scheme:  "Bearer",
					FromCtx: bearertoken.GetBearerToken,
				},
			},
			requestContext:     context.Background(),
			expectNoAuthHeader: true,
		},
		{
			name:           "Nil transport - should return error",
			auths:          []Method{},
			requestContext: context.Background(),
			expectError:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // Enable parallel execution for faster tests

			// Each test gets its own isolated capturedRequest and transport
			var capturedRequest *http.Request
			wrappedTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				capturedRequest = r
				return &http.Response{StatusCode: http.StatusOK}, nil
			})

			req, err := http.NewRequestWithContext(tc.requestContext, http.MethodGet, "http://fake.example.com/api", nil)
			require.NoError(t, err)

			var transport http.RoundTripper = wrappedTransport
			if tc.expectError {
				transport = nil // Force nil transport for error test
			}

			tr := RoundTripper{
				Transport: transport,
				Auths:     tc.auths,
			}

			resp, err := tr.RoundTrip(req)

			if tc.expectError {
				assert.Nil(t, resp)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no http.RoundTripper provided")
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
			require.NotNil(t, capturedRequest, "Request should have been captured")

			// Perform assertions on captured request
			if tc.expectNoAuthHeader {
				assert.Empty(t, capturedRequest.Header.Get("Authorization"))
			} else if len(tc.expectedHeaders) > 0 {
				authHeaders := capturedRequest.Header["Authorization"]
				assert.ElementsMatch(t, tc.expectedHeaders, authHeaders)
			}
		})
	}
}
