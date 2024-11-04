// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tenancy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHttpHandler struct {
	reached bool
}

func (thh *testHttpHandler) ServeHTTP(http.ResponseWriter, *http.Request) {
	thh.reached = true
}

func TestProgationHandler(t *testing.T) {
	tests := []struct {
		name           string
		tenancyMgr     *Manager
		shouldReach    bool
		requestHeaders map[string][]string
	}{
		{
			name:           "untenanted",
			tenancyMgr:     NewManager(&Options{}),
			requestHeaders: map[string][]string{},
			shouldReach:    true,
		},
		{
			name:           "missing tenant header",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{},
			shouldReach:    false,
		},
		{
			name:           "valid tenant header",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {"acme"}},
			shouldReach:    true,
		},
		{
			name:           "unauthorized tenant",
			tenancyMgr:     NewManager(&Options{Enabled: true, Tenants: []string{"megacorp"}}),
			requestHeaders: map[string][]string{"x-tenant": {"acme"}},
			shouldReach:    false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			handler := &testHttpHandler{}
			propH := ExtractTenantHTTPHandler(test.tenancyMgr, handler)
			req, err := http.NewRequest(http.MethodGet, "/", strings.NewReader(""))
			for k, vs := range test.requestHeaders {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}
			require.NoError(t, err)
			writer := httptest.NewRecorder()
			propH.ServeHTTP(writer, req)
			assert.Equal(t, test.shouldReach, handler.reached)
		})
	}
}

func TestMetadataAnnotator(t *testing.T) {
	tests := []struct {
		name           string
		tenancyMgr     *Manager
		requestHeaders map[string][]string
	}{
		{
			name:           "missing tenant",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{},
		},
		{
			name:           "tenanted",
			tenancyMgr:     NewManager(&Options{Enabled: true}),
			requestHeaders: map[string][]string{"x-tenant": {"acme"}},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest(http.MethodGet, "/", strings.NewReader(""))
			for k, vs := range test.requestHeaders {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}
			require.NoError(t, err)
			annotator := test.tenancyMgr.MetadataAnnotator()
			md := annotator(context.Background(), req)
			assert.Equal(t, len(test.requestHeaders), len(md))
		})
	}
}
