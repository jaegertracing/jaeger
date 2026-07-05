// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
)

func TestExists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		responseCode   int
		response       string
		errContains    string
		expectedResult bool
	}{
		{
			name:           "found",
			responseCode:   http.StatusOK,
			expectedResult: true,
		},
		{
			name:         "not found",
			responseCode: http.StatusNotFound,
			response:     esErrResponse,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to get ILM policy: jaeger-ilm-policy",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), "_ilm/policy/jaeger-ilm-policy"))
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, testBasicAuthHeader, req.Header.Get("Authorization"))
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			client := makeClient(t, testServer.URL, "user", "pass")
			client.Version = es.ElasticV7
			c := &ILMClient{
				Client: client,
				Logger: zap.NewNop(),
			}
			result, err := c.Exists(context.Background(), "jaeger-ilm-policy")
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)
			}
			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestExists_OpenSearchISM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		responseCode   int
		response       string
		errContains    string
		expectedResult bool
	}{
		{
			name:           "found",
			responseCode:   http.StatusOK,
			expectedResult: true,
		},
		{
			name:         "not found",
			responseCode: http.StatusNotFound,
			response:     esErrResponse,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to get ILM policy: jaeger-ilm-policy",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), "_plugins/_ism/policies/jaeger-ilm-policy"))
				assert.Equal(t, http.MethodGet, req.Method)
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			client := makeClient(t, testServer.URL, "", "")
			client.Version = es.OpenSearch2
			c := &ILMClient{
				Client: client,
				Logger: zap.NewNop(),
			}
			result, err := c.Exists(context.Background(), "jaeger-ilm-policy")
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)
			}
			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestExists_Retries(t *testing.T) {
	t.Parallel()
	var callCount int

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		callCount++
		assert.True(t, strings.HasSuffix(req.URL.String(), "_ilm/policy/jaeger-ilm-policy"))
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, testBasicAuthHeader, req.Header.Get("Authorization"))

		if callCount < maxTries {
			res.WriteHeader(http.StatusInternalServerError)
			res.Write([]byte(`{"error": "server error"}`))
			return
		}

		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	client := makeClient(t, testServer.URL, "user", "pass")
	client.Version = es.ElasticV7
	c := &ILMClient{
		Client: client,
		Logger: zap.NewNop(),
	}

	result, err := c.Exists(context.Background(), "jaeger-ilm-policy")
	require.NoError(t, err)
	assert.True(t, result)
	assert.Equal(t, maxTries, callCount, "should retry twice before succeeding")
}

// TestLifecycleExistsRequestSnapshot freezes the wire format of the ILM/ISM
// policy-existence probe: ES uses _ilm/policy, OpenSearch uses _plugins/_ism.
func TestLifecycleExistsRequestSnapshot(t *testing.T) {
	content := map[es.BackendVersion]string{}
	for _, version := range es.AllVersions {
		rec, url := okServer(t)
		client := makeClient(t, url, "", "")
		client.Version = version
		c := ILMClient{
			Client: client,
			Logger: zap.NewNop(),
		}
		_, err := c.Exists(context.Background(), "jaeger-ilm-policy")
		require.NoError(t, err)
		content[version] = rec.Marshal(t)
	}
	snapshottest.AssertByVersion(t, "testdata/ilm_exists", content)
}
