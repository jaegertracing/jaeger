// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
)

const badVersionType = `
{
	"name" : "opensearch-node1",
	"cluster_name" : "opensearch-cluster",
	"cluster_uuid" : "1StaUGrGSx61r41d-1nDiw",
	"version" : {
	  "distribution" : "opensearch",
	  "number" : true,
	  "build_type" : "tar",
	  "build_hash" : "34550c5b17124ddc59458ef774f6b43a086522e3",
	  "build_date" : "2021-07-02T23:22:21.383695Z",
	  "build_snapshot" : false,
	  "lucene_version" : "8.8.2",
	  "minimum_wire_compatibility_version" : "6.8.0",
	  "minimum_index_compatibility_version" : "6.0.0-beta1"
	},
	"tagline" : "The OpenSearch Project: https://opensearch.org/"
  }
`

const badVersionNoNumber = `
{
	"name" : "opensearch-node1",
	"cluster_name" : "opensearch-cluster",
	"cluster_uuid" : "1StaUGrGSx61r41d-1nDiw",
	"version" : {
	  "distribution" : "opensearch",
	  "number" : "thisisnotanumber",
	  "build_type" : "tar",
	  "build_hash" : "34550c5b17124ddc59458ef774f6b43a086522e3",
	  "build_date" : "2021-07-02T23:22:21.383695Z",
	  "build_snapshot" : false,
	  "lucene_version" : "8.8.2",
	  "minimum_wire_compatibility_version" : "6.8.0",
	  "minimum_index_compatibility_version" : "6.0.0-beta1"
	},
	"tagline" : "The OpenSearch Project: https://opensearch.org/"
  }
`

const opensearch1 = `
{
	"name" : "opensearch-node1",
	"cluster_name" : "opensearch-cluster",
	"cluster_uuid" : "1StaUGrGSx61r41d-1nDiw",
	"version" : {
	  "distribution" : "opensearch",
	  "number" : "1.0.0",
	  "build_type" : "tar",
	  "build_hash" : "34550c5b17124ddc59458ef774f6b43a086522e3",
	  "build_date" : "2021-07-02T23:22:21.383695Z",
	  "build_snapshot" : false,
	  "lucene_version" : "8.8.2",
	  "minimum_wire_compatibility_version" : "6.8.0",
	  "minimum_index_compatibility_version" : "6.0.0-beta1"
	},
	"tagline" : "The OpenSearch Project: https://opensearch.org/"
  }
`

const opensearch2 = `
{
	"name" : "opensearch-node1",
	"cluster_name" : "opensearch-cluster",
	"cluster_uuid" : "1StaUGrGSx61r41d-1nDiw",
	"version" : {
	  "distribution" : "opensearch",
	  "number" : "2.3.0",
	  "build_type" : "tar",
	  "build_hash" : "34550c5b17124ddc59458ef774f6b43a086522e3",
	  "build_date" : "2021-07-02T23:22:21.383695Z",
	  "build_snapshot" : false,
	  "lucene_version" : "8.8.2",
	  "minimum_wire_compatibility_version" : "6.8.0",
	  "minimum_index_compatibility_version" : "6.0.0-beta1"
	},
	"tagline" : "The OpenSearch Project: https://opensearch.org/"
  }
`

const opensearch3 = `
{
	"name" : "opensearch-node1",
	"cluster_name" : "opensearch-cluster",
	"cluster_uuid" : "1StaUGrGSx61r41d-1nDiw",
	"version" : {
	  "distribution" : "opensearch",
	  "number" : "3.0.0",
	  "build_type" : "tar",
	  "build_hash" : "34550c5b17124ddc59458ef774f6b43a086522e3",
	  "build_date" : "2025-05-06T23:22:21.383695Z",
	  "build_snapshot" : false,
	  "lucene_version" : "10.0.0",
	  "minimum_wire_compatibility_version" : "6.8.0",
	  "minimum_index_compatibility_version" : "6.0.0-beta1"
	},
	"tagline" : "The OpenSearch Project: https://opensearch.org/"
  }
`

const elasticsearch7 = `
{
	"name" : "elasticsearch-0",
	"cluster_name" : "clustername",
	"cluster_uuid" : "HUtdg7bRTomSFaOk7Wzt8w",
	"version" : {
	  "number" : "7.6.1",
	  "build_flavor" : "default",
	  "build_type" : "docker",
	  "build_hash" : "aa751e09be0a5072e8570670309b1f12348f023b",
	  "build_date" : "2020-02-29T00:15:25.529771Z",
	  "build_snapshot" : false,
	  "lucene_version" : "8.4.0",
	  "minimum_wire_compatibility_version" : "6.8.0",
	  "minimum_index_compatibility_version" : "6.0.0-beta1"
	},
	"tagline" : "You Know, for Search"
  }
`

const elasticsearch8 = `
{
	"name" : "elasticsearch-0",
	"version" : {
	  "number" : "8.0.0"
	},
	"tagline" : "You Know, for Search"
  }
`

const elasticsearch6 = `
{
	"name" : "elasticsearch-0",
	"cluster_name" : "clustername",
	"cluster_uuid" : "HUtdg7bRTomSFaOk7Wzt8w",
	"version" : {
	  "number" : "6.8.0",
	  "build_flavor" : "default",
	  "build_type" : "docker",
	  "build_hash" : "aa751e09be0a5072e8570670309b1f12348f023b",
	  "build_date" : "2020-02-29T00:15:25.529771Z",
	  "build_snapshot" : false,
	  "lucene_version" : "8.4.0",
	  "minimum_wire_compatibility_version" : "6.8.0",
	  "minimum_index_compatibility_version" : "6.0.0-beta1"
	},
	"tagline" : "You Know, for Search"
  }
`

func TestVersion(t *testing.T) {
	tests := []struct {
		name           string
		responseCode   int
		response       string
		errContains    string
		expectedResult es.BackendVersion
	}{
		{
			name:           "success with elasticsearch 6",
			responseCode:   http.StatusOK,
			response:       elasticsearch6,
			expectedResult: es.ElasticV6,
		},
		{
			name:           "success with elasticsearch 7",
			responseCode:   http.StatusOK,
			response:       elasticsearch7,
			expectedResult: es.ElasticV7,
		},
		{
			name:           "success with elasticsearch 8",
			responseCode:   http.StatusOK,
			response:       elasticsearch8,
			expectedResult: es.ElasticV8,
		},
		{
			name:           "success with opensearch 1",
			responseCode:   http.StatusOK,
			response:       opensearch1,
			expectedResult: es.OpenSearch1,
		},
		{
			name:           "success with opensearch 2",
			responseCode:   http.StatusOK,
			response:       opensearch2,
			expectedResult: es.OpenSearch2,
		},
		{
			name:           "success with opensearch 3",
			responseCode:   http.StatusOK,
			response:       opensearch3,
			expectedResult: es.OpenSearch3,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "request failed, status code: 400",
		},
		{
			name:         "bad version",
			responseCode: http.StatusOK,
			response:     badVersionType,
			errContains:  "invalid version format: true",
		},
		{
			name:         "version not a number",
			responseCode: http.StatusOK,
			response:     badVersionNoNumber,
			errContains:  "invalid version format: thisisnotanumber",
		},
		{
			name:         "unmarshal error",
			responseCode: http.StatusOK,
			response:     "thisisaninvalidjson",
			errContains:  "invalid character",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, testBasicAuthHeader, req.Header.Get("Authorization"))
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			// Version 0 (unset) makes NewClient probe the cluster (GET /) and
			// resolve the version at construction.
			c, err := NewClient(context.Background(), &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: configoptional.Some(config.BasicAuthentication{Username: "user", Password: "pass"}),
				},
			}, zap.NewNop(), nil)
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expectedResult, c.version)
		})
	}
}

// versionResponse returns a cluster-info body that DetectBackendVersion maps back
// to the given BackendVersion. Shared by the request-snapshot tests (e.g. the
// CreateTemplate probe in index_client_test.go).
func versionResponse(v es.BackendVersion) string {
	number := map[es.BackendVersion]string{
		es.ElasticV6:   "6.8.0",
		es.ElasticV7:   "7.10.2",
		es.ElasticV8:   "8.0.0",
		es.ElasticV9:   "9.0.0",
		es.OpenSearch1: "1.3.0",
		es.OpenSearch2: "2.11.0",
		es.OpenSearch3: "3.0.0",
	}
	tagline := "You Know, for Search"
	if v.IsOpenSearch() {
		tagline = "The OpenSearch Project: https://opensearch.org/"
	}
	return fmt.Sprintf(`{"version":{"number":%q},"tagline":%q}`, number[v], tagline)
}

// TestVersionRequestSnapshot freezes the wire format of the version probe. The
// request (GET /) is identical for every backend, so it has a single snapshot
// shared across all versions.
func TestVersionRequestSnapshot(t *testing.T) {
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(versionResponse(es.ElasticV7)))
	})
	server := httptest.NewServer(rec)
	defer server.Close()

	_, err := NewClient(context.Background(), &config.Configuration{Servers: []string{server.URL}}, zap.NewNop(), nil)
	require.NoError(t, err)
	rec.Assert(t, "testdata/version")
}

// TestResolveVersionHonorsConfigured verifies that an explicit configured
// version is returned without probing the cluster.
func TestResolveVersionHonorsConfigured(t *testing.T) {
	var pinged bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		pinged = true
	}))
	defer server.Close()

	c, err := NewClient(context.Background(), &config.Configuration{
		Servers: []string{server.URL},
		Version: uint(es.OpenSearch2),
	}, zap.NewNop(), nil)
	require.NoError(t, err)
	assert.Equal(t, es.OpenSearch2, c.version)
	assert.False(t, pinged, "configured version must not trigger a cluster probe")
}
