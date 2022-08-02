// Copyright (c) 2022 The Jaeger Authors.
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

func (thh *testHttpHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
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
		t.Run(test.name, func(t *testing.T) {
			handler := &testHttpHandler{}
			propH := ExtractTenantHTTPHandler(test.tenancyMgr, handler)
			req, err := http.NewRequest("GET", "/", strings.NewReader(""))
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
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/", strings.NewReader(""))
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
