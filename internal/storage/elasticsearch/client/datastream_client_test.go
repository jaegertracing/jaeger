// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCreateComponentTemplate(t *testing.T) {
	tests := []struct {
		name         string
		responseCode int
		expectedErr  string
	}{
		{name: "success", responseCode: http.StatusOK},
		{name: "failure", responseCode: http.StatusBadRequest, expectedErr: "failed to create component template: jaeger.spans@mappings"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotMethod, gotPath string
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				gotMethod, gotPath = req.Method, req.URL.Path
				res.WriteHeader(test.responseCode)
			}))
			defer testServer.Close()
			c := &IndicesClient{Client: Client{Client: testServer.Client(), Endpoint: testServer.URL}}

			err := c.CreateComponentTemplate(context.Background(), "jaeger.spans@mappings", `{"template":{}}`)

			assert.Equal(t, http.MethodPut, gotMethod)
			assert.Equal(t, "/_component_template/jaeger.spans@mappings", gotPath)
			if test.expectedErr != "" {
				require.ErrorContains(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateIndexTemplate_AlwaysComposable(t *testing.T) {
	var gotPath string
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()
	c := &IndicesClient{Client: Client{Client: testServer.Client(), Endpoint: testServer.URL}}

	require.NoError(t, c.CreateIndexTemplate(context.Background(), "jaeger.spans", `{"data_stream":{}}`))
	// Must use the composable _index_template API, not the legacy _template API,
	// regardless of server version (data streams require composable templates).
	assert.Equal(t, "/_index_template/jaeger.spans", gotPath)
}

func TestISMClient_Exists(t *testing.T) {
	tests := []struct {
		name         string
		responseCode int
		wantExists   bool
		expectedErr  string
	}{
		{name: "exists", responseCode: http.StatusOK, wantExists: true},
		{name: "not found", responseCode: http.StatusNotFound, wantExists: false},
		{name: "server error", responseCode: http.StatusInternalServerError, expectedErr: "failed to get ISM policy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotMethod, gotPath string
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				gotMethod, gotPath = req.Method, req.URL.Path
				res.WriteHeader(test.responseCode)
			}))
			defer testServer.Close()
			c := ISMClient{Client: Client{Client: testServer.Client(), Endpoint: testServer.URL}, Logger: zap.NewNop()}

			exists, err := c.Exists(context.Background(), "jaeger-spans-policy")

			assert.Equal(t, http.MethodGet, gotMethod)
			assert.Equal(t, "/_plugins/_ism/policies/jaeger-spans-policy", gotPath)
			if test.expectedErr != "" {
				require.ErrorContains(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.wantExists, exists)
			}
		})
	}
}

func TestISMClient_Create(t *testing.T) {
	var gotMethod, gotPath string
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		gotMethod, gotPath = req.Method, req.URL.Path
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()
	c := ISMClient{Client: Client{Client: testServer.Client(), Endpoint: testServer.URL}, Logger: zap.NewNop()}

	require.NoError(t, c.Create(context.Background(), "jaeger-spans-policy", `{"policy":{}}`))
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/_plugins/_ism/policies/jaeger-spans-policy", gotPath)
}

func TestISMClient_Create_Error(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusBadRequest)
	}))
	defer testServer.Close()
	c := ISMClient{Client: Client{Client: testServer.Client(), Endpoint: testServer.URL}, Logger: zap.NewNop()}

	require.ErrorContains(t, c.Create(context.Background(), "jaeger-spans-policy", `{}`), "failed to create ISM policy: jaeger-spans-policy")
}

func TestILMClient_Create(t *testing.T) {
	var gotMethod, gotPath string
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		gotMethod, gotPath = req.Method, req.URL.Path
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()
	c := ILMClient{Client: Client{Client: testServer.Client(), Endpoint: testServer.URL}, Logger: zap.NewNop()}

	require.NoError(t, c.Create(context.Background(), "jaeger-spans-policy", `{"policy":{}}`))
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/_ilm/policy/jaeger-spans-policy", gotPath)
}
