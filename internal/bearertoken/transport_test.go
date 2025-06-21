// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}

func TestCloneRequest(t *testing.T) {
	testURL, _ := url.Parse("http://example.com/path?query=value")
	headers := http.Header{
		"Content-Type":  {"application/json"},
		"X-Test-Header": {"test-value"},
	}

	tests := []struct {
		name    string
		request *http.Request
	}{
		{
			name: "with headers and URL",
			request: &http.Request{
				Method: http.MethodPost,
				URL:    testURL,
				Header: headers,
			},
		},
		{
			name: "nil URL",
			request: &http.Request{
				Method: http.MethodGet,
				Header: http.Header{"X-Test": {"value"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := cloneRequest(tt.request)

			// Check basic fields
			if clone.Method != tt.request.Method {
				t.Errorf("Method not cloned correctly: got %v, want %v", clone.Method, tt.request.Method)
			}

			// Check URL
			if tt.request.URL == nil && clone.URL != nil {
				t.Error("Expected nil URL in clone when original is nil")
			} else if tt.request.URL != nil {
				if clone.URL == nil {
					t.Error("Expected URL to be cloned, got nil")
				} else if clone.URL.String() != tt.request.URL.String() {
					t.Errorf("URL not cloned correctly: got %v, want %v", clone.URL, tt.request.URL)
				}
			}

			// Check headers
			if len(clone.Header) != len(tt.request.Header) {
				t.Errorf("Header length mismatch: got %d, want %d", len(clone.Header), len(tt.request.Header))
			}

			for k, v := range tt.request.Header {
				got := clone.Header[k]
				if len(got) != len(v) {
					t.Errorf("Header %s length mismatch: got %d, want %d", k, len(got), len(v))
					continue
				}
				for i := range v {
					if got[i] != v[i] {
						t.Errorf("Header %s[%d] = %q, want %q", k, i, got[i], v[i])
					}
				}
			}

			// Verify it's a deep copy by modifying the clone and checking original is unchanged
			if len(tt.request.Header) > 0 {
				origHeaderCount := len(tt.request.Header)
				clone.Header.Add("X-New-Header", "test")
				if len(tt.request.Header) != origHeaderCount {
					t.Error("Modifying clone header modified original header")
				}
			}
		})
	}
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
