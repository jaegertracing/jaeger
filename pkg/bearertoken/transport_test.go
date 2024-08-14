// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
			name:            "Override from context provided, and request context set should use request context token",
			overrideFromCtx: true,
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer tokenFromContext", r.Header.Get("Authorization"))
				return &http.Response{
					StatusCode: http.StatusOK,
				}, nil
			}),
			requestContext: ContextWithBearerToken(context.Background(), "tokenFromContext"),
		},
		{
			name:            "Allow override from context and token provided, and request context unset should use defaultToken",
			overrideFromCtx: true,
			staticToken:     "initToken",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer initToken", r.Header.Get("Authorization"))
				return &http.Response{}, nil
			}),
			requestContext: context.Background(),
		},
		{
			name:            "Allow override from context and token provided, and request context set should use context token",
			overrideFromCtx: true,
			staticToken:     "initToken",
			wrappedTransport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer tokenFromContext", r.Header.Get("Authorization"))
				return &http.Response{}, nil
			}),
			requestContext: ContextWithBearerToken(context.Background(), "tokenFromContext"),
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
			req, err := http.NewRequestWithContext(tc.requestContext, "GET", server.URL, nil)
			require.NoError(t, err)

			tr := RoundTripper{
				Transport:       tc.wrappedTransport,
				OverrideFromCtx: tc.overrideFromCtx,
				StaticToken:     tc.staticToken,
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
