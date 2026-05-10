// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPServerMiddleware(t *testing.T) {
	headers := []headerforwarding.ForwardedHeader{
		{HTTPName: "x-user", Role: headerforwarding.RoleUsername},
		{HTTPName: "x-email", Role: headerforwarding.RoleEmail},
	}

	tests := []struct {
		name     string
		reqHdrs  map[string]string
		wantVals map[string]string // httpName -> value
	}{
		{
			name:     "no headers",
			reqHdrs:  nil,
			wantVals: nil,
		},
		{
			name:     "one matching header",
			reqHdrs:  map[string]string{"x-user": "alice"},
			wantVals: map[string]string{"x-user": "alice"},
		},
		{
			name:     "both headers",
			reqHdrs:  map[string]string{"x-user": "alice", "x-email": "alice@example.com"},
			wantVals: map[string]string{"x-user": "alice", "x-email": "alice@example.com"},
		},
		{
			name:     "unknown header ignored",
			reqHdrs:  map[string]string{"x-other": "value"},
			wantVals: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotVals map[string]string
			inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				captured := headerforwarding.CapturedFromContext(r.Context())
				if captured != nil {
					gotVals = make(map[string]string)
					for _, c := range captured {
						gotVals[c.Header.HTTPName] = c.Value
					}
				}
			})
			h := headerforwarding.HTTPServerMiddleware(headers, inner)

			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			for k, v := range tt.reqHdrs {
				req.Header.Set(k, v)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tt.wantVals, gotVals)
		})
	}
}

func TestHTTPRoundTripper(t *testing.T) {
	headers := []headerforwarding.ForwardedHeader{
		{
			HTTPName:         "x-user",
			GRPCOutboundName: "x-storage-user",
			Role:             headerforwarding.RoleUsername,
		},
		{
			HTTPName: "x-email",
			Role:     headerforwarding.RoleEmail,
		},
	}

	var gotUser, gotEmail string
	rt := headerforwarding.NewHTTPRoundTripper(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		gotUser = req.Header.Get("x-storage-user")
		gotEmail = req.Header.Get("x-email")
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: &headers[0], Value: "alice"},
		{Header: &headers[1], Value: "alice@example.com"},
	})
	req = req.WithContext(ctx)

	resp, err := rt.RoundTrip(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "alice", gotUser)
	assert.Equal(t, "alice@example.com", gotEmail)
	assert.Empty(t, req.Header.Get("x-storage-user"))
	assert.Empty(t, req.Header.Get("x-email"))
}
