// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

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
