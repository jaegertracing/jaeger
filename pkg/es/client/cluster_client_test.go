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

package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

const opensearchInfo = `
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
		expectedResult uint
	}{
		{
			name:           "success with elasticsearch 6",
			responseCode:   http.StatusOK,
			response:       elasticsearch6,
			expectedResult: 6,
		},
		{
			name:           "success with elasticsearch 7",
			responseCode:   http.StatusOK,
			response:       elasticsearch7,
			expectedResult: 7,
		},
		{
			name:           "success with opensearch",
			responseCode:   http.StatusOK,
			response:       opensearchInfo,
			expectedResult: 7,
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
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &ClusterClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			result, err := c.Version()
			if test.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errContains)
			}
			assert.Equal(t, test.expectedResult, result)
		})
	}
}
