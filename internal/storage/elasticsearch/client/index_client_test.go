// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const esIndexResponse = `
{
  "%sjaeger-service-2021-08-06" : {
    "aliases" : { },
    "settings" : {
      "index.creation_date" : "1628259381266",
      "index.mapper.dynamic" : "false",
      "index.mapping.nested_fields.limit" : "50",
      "index.number_of_replicas" : "1",
      "index.number_of_shards" : "5",
      "index.provided_name" : "jaeger-service-2021-08-06",
      "index.requests.cache.enable" : "true",
      "index.uuid" : "2kKdvrvAT7qXetRzmWhjYQ",
      "index.version.created" : "5061099"
    }
  },
  "%sjaeger-span-2021-08-06" : {
    "aliases" : { },
    "settings" : {
      "index.creation_date" : "1628259381326",
      "index.mapper.dynamic" : "false",
      "index.mapping.nested_fields.limit" : "50",
      "index.number_of_replicas" : "1",
      "index.number_of_shards" : "5",
      "index.provided_name" : "jaeger-span-2021-08-06",
      "index.requests.cache.enable" : "true",
      "index.uuid" : "zySRY_FfRFa5YMWxNsNViA",
      "index.version.created" : "5061099"
    }
  },
 "%sjaeger-span-000001" : {
    "aliases" : {
      "jaeger-span-read" : { },
      "jaeger-span-write" : { }
    },
    "settings" : {
      "index.creation_date" : "1628259381326"
    }
  }
}`

const esErrResponse = `{"error":{"root_cause":[{"type":"illegal_argument_exception","reason":"request [/jaeger-*] contains unrecognized parameter: [help]"}],"type":"illegal_argument_exception","reason":"request [/jaeger-*] contains unrecognized parameter: [help]"},"status":400}`

func TestClientGetIndices(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		responseCode int
		response     string
		errContains  string
		indices      []Index
	}{
		{
			name:         "no error",
			responseCode: http.StatusOK,
			response:     esIndexResponse,
			indices: []Index{
				{
					Index:        "jaeger-service-2021-08-06",
					CreationTime: time.Unix(0, int64(time.Millisecond)*1628259381266),
					Aliases:      map[string]bool{},
				},
				{
					Index:        "jaeger-span-000001",
					CreationTime: time.Unix(0, int64(time.Millisecond)*1628259381326),
					Aliases:      map[string]bool{"jaeger-span-read": true, "jaeger-span-write": true},
				},
				{
					Index:        "jaeger-span-2021-08-06",
					CreationTime: time.Unix(0, int64(time.Millisecond)*1628259381326),
					Aliases:      map[string]bool{},
				},
			},
		},
		{
			name:         "no error with prefix",
			prefix:       "foo-",
			responseCode: http.StatusOK,
			response:     esIndexResponse,
			indices: []Index{
				{
					Index:        "foo-jaeger-service-2021-08-06",
					CreationTime: time.Unix(0, int64(time.Millisecond)*1628259381266),
					Aliases:      map[string]bool{},
				},
				{
					Index:        "foo-jaeger-span-000001",
					CreationTime: time.Unix(0, int64(time.Millisecond)*1628259381326),
					Aliases:      map[string]bool{"jaeger-span-read": true, "jaeger-span-write": true},
				},
				{
					Index:        "foo-jaeger-span-2021-08-06",
					CreationTime: time.Unix(0, int64(time.Millisecond)*1628259381326),
					Aliases:      map[string]bool{},
				},
			},
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to query indices: request failed, status code: 400",
		},
		{
			name:         "unmarshall error",
			responseCode: http.StatusOK,
			response:     "AAA",
			errContains:  `failed to query indices and unmarshall response body: "AAA"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
				res.WriteHeader(test.responseCode)

				response := test.response
				if test.errContains == "" {
					// Formatted string only applies to "success" response bodies.
					response = fmt.Sprintf(test.response, test.prefix, test.prefix, test.prefix)
				}
				res.Write([]byte(response))
			}))
			defer testServer.Close()

			c := &IndicesClient{
				Client: Client{
					Client:   testServer.Client(),
					Endpoint: testServer.URL,
				},
			}

			indices, err := c.GetJaegerIndices(test.prefix)
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)
				assert.Nil(t, indices)
			} else {
				require.NoError(t, err)
				sort.Slice(indices, func(i, j int) bool {
					return strings.Compare(indices[i].Index, indices[j].Index) < 0
				})
				assert.Equal(t, test.indices, indices)
			}
		})
	}
}

func getIndicesList(size int) []Index {
	indicesList := []Index{}
	for count := 1; count <= size/2; count++ {
		indicesList = append(indicesList, Index{Index: fmt.Sprintf("jaeger-span-%06d", count)})
		indicesList = append(indicesList, Index{Index: fmt.Sprintf("jaeger-service-%06d", count)})
	}
	return indicesList
}

func TestClientDeleteIndices(t *testing.T) {
	masterTimeoutSeconds := 1
	maxURLPathLength := 4000
	ignoreUnavailableIndex := true

	tests := []struct {
		name         string
		responseCode int
		response     string
		errContains  string
		indices      []Index
		triggerAPI   bool
	}{
		{
			name:         "no indices",
			responseCode: http.StatusOK,
			indices:      []Index{},
			triggerAPI:   false,
		},
		{
			name:         "one index",
			responseCode: http.StatusOK,
			indices:      []Index{{Index: "jaeger-span-000001"}},
			triggerAPI:   true,
		},
		{
			name:         "moderate indices",
			responseCode: http.StatusOK,
			response:     "",
			indices:      getIndicesList(20),
			triggerAPI:   true,
		},
		{
			name:         "long indices",
			responseCode: http.StatusOK,
			response:     "",
			indices:      getIndicesList(600),
			triggerAPI:   true,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to delete indices: jaeger-span-000001",
			indices:      []Index{{Index: "jaeger-span-000001"}},
			triggerAPI:   true,
		},
		{
			name:         "client error in long indices",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to delete indices: jaeger-span-000001",
			indices:      getIndicesList(600),
			triggerAPI:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deletedIndicesCount := 0
			apiTriggered := false
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				apiTriggered = true
				assert.Equal(t, http.MethodDelete, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				assert.Equal(t, fmt.Sprintf("%ds", masterTimeoutSeconds), req.URL.Query().Get("master_timeout"))
				assert.Equal(t, strconv.FormatBool(ignoreUnavailableIndex), req.URL.Query().Get("ignore_unavailable"))
				assert.LessOrEqual(t, len(req.URL.Path), maxURLPathLength)

				// removes leading '/' and trailing ','
				// example: /jaeger-span-000001,  =>  jaeger-span-000001
				rawIndices := strings.TrimPrefix(req.URL.Path, "/")
				rawIndices = strings.TrimSuffix(rawIndices, ",")

				if len(test.indices) == 1 {
					assert.Equal(t, test.indices[0].Index, rawIndices)
				}

				deletedIndices := strings.Split(rawIndices, ",")
				deletedIndicesCount += len(deletedIndices)

				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
				MasterTimeoutSeconds:   masterTimeoutSeconds,
				IgnoreUnavailableIndex: ignoreUnavailableIndex,
			}

			err := c.DeleteIndices(test.indices)
			assert.Equal(t, test.triggerAPI, apiTriggered)

			if test.errContains != "" {
				assert.ErrorContains(t, err, test.errContains)
			} else {
				assert.Len(t, test.indices, deletedIndicesCount)
			}
		})
	}
}

func TestIndexExists(t *testing.T) {
	t.Run("index exists", func(t *testing.T) {
		testIndexOrAliasExistence(t, "index")
	})
}

func TestAliasExists(t *testing.T) {
	t.Run("alias exists", func(t *testing.T) {
		testIndexOrAliasExistence(t, "alias")
	})
}

func testIndexOrAliasExistence(t *testing.T, existence string) {
	maxURLPathLength := 4000
	type indexOrAliasExistence struct {
		name         string
		exists       bool
		responseCode int
		expectedErr  string
	}
	tests := []indexOrAliasExistence{
		{
			name:         "exists",
			responseCode: http.StatusOK,
			exists:       true,
		},
		{
			name:         "not exists",
			responseCode: http.StatusNotFound,
			exists:       false,
		},
	}
	if existence == "index" {
		test := indexOrAliasExistence{
			name:         "generic error",
			responseCode: http.StatusBadRequest,
			expectedErr:  "failed to check if index exists: request failed, status code: 400",
		}
		tests = append(tests, test)
	} else if existence == "alias" {
		test := indexOrAliasExistence{
			name:         "generic error",
			responseCode: http.StatusBadRequest,
			expectedErr:  "failed to check if alias exists: request failed, status code: 400",
		}
		tests = append(tests, test)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiTriggered := false
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				apiTriggered = true
				assert.Equal(t, http.MethodHead, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				assert.LessOrEqual(t, len(req.URL.Path), maxURLPathLength)
				res.WriteHeader(test.responseCode)
			}))
			defer testServer.Close()
			c := &IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			var exists bool
			var err error
			if existence == "index" {
				exists, err = c.IndexExists("jaeger-span")
			} else if existence == "alias" {
				exists, err = c.AliasExists("jaeger-span")
			}
			if test.expectedErr != "" {
				require.ErrorContains(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
			}
			assert.True(t, apiTriggered)
			assert.Equal(t, test.exists, exists)
		})
	}
}

func TestClientRequestError(t *testing.T) {
	c := &IndicesClient{
		Client: Client{
			Endpoint: "%",
		},
	}
	indices, err := c.GetJaegerIndices("")
	require.Error(t, err)
	assert.Nil(t, indices)
}

func TestClientDoError(t *testing.T) {
	c := &IndicesClient{
		Client: Client{
			Client:   &http.Client{},
			Endpoint: "localhost:1",
		},
	}

	indices, err := c.GetJaegerIndices("")
	require.Error(t, err)
	assert.Nil(t, indices)
}

func TestClientCreateIndex(t *testing.T) {
	indexName := "jaeger-span"
	tests := []struct {
		name         string
		responseCode int
		response     string
		errContains  string
	}{
		{
			name:         "success",
			responseCode: http.StatusOK,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to create index: jaeger-span",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), "jaeger-span"))
				assert.Equal(t, http.MethodPut, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			err := c.CreateIndex(indexName)
			if test.errContains != "" {
				assert.ErrorContains(t, err, test.errContains)
			}
		})
	}
}

func TestClientCreateAliases(t *testing.T) {
	aliases := []Alias{
		{
			Index: "jaeger-span",
			Name:  "jaeger-span-read",
		},
		{
			Index:        "jaeger-span",
			Name:         "jaeger-span-write",
			IsWriteIndex: true,
		},
	}
	expectedRequestBody := `{"actions":[{"add":{"alias":"jaeger-span-read","index":"jaeger-span"}},{"add":{"alias":"jaeger-span-write","index":"jaeger-span","is_write_index":true}}]}`
	tests := []struct {
		name         string
		responseCode int
		response     string
		errContains  string
	}{
		{
			name:         "success",
			responseCode: http.StatusOK,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to create aliases: [index: jaeger-span, alias: jaeger-span-read],[index: jaeger-span, alias: jaeger-span-write]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), "_aliases"))
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				body, err := io.ReadAll(req.Body)
				if assert.NoError(t, err) {
					assert.Equal(t, expectedRequestBody, string(body))
					res.WriteHeader(test.responseCode)
					res.Write([]byte(test.response))
				}
			}))
			defer testServer.Close()

			c := &IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			err := c.CreateAlias(aliases)
			if test.errContains != "" {
				assert.ErrorContains(t, err, test.errContains)
			}
		})
	}
}

func TestClientDeleteAliases(t *testing.T) {
	aliases := []Alias{
		{
			Index: "jaeger-span",
			Name:  "jaeger-span-read",
		},
		{
			Index:        "jaeger-span",
			Name:         "jaeger-span-write",
			IsWriteIndex: true,
		},
	}
	expectedRequestBody := `{"actions":[{"remove":{"alias":"jaeger-span-read","index":"jaeger-span"}},{"remove":{"alias":"jaeger-span-write","index":"jaeger-span","is_write_index":true}}]}`
	tests := []struct {
		name         string
		responseCode int
		response     string
		errContains  string
	}{
		{
			name:         "success",
			responseCode: http.StatusOK,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to delete aliases: [index: jaeger-span, alias: jaeger-span-read],[index: jaeger-span, alias: jaeger-span-write]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), "_aliases"))
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				body, err := io.ReadAll(req.Body)
				assert.NoError(t, err)
				assert.Equal(t, expectedRequestBody, string(body))
				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			err := c.DeleteAlias(aliases)
			if test.errContains != "" {
				assert.ErrorContains(t, err, test.errContains)
			}
		})
	}
}

func TestClientCreateTemplate(t *testing.T) {
	templateName := "jaeger-template"
	templateContent := "template content"
	tests := []struct {
		name         string
		versionResp  string
		responseCode int
		response     string
		errContains  string
	}{
		{
			name:         "success/v7",
			versionResp:  elasticsearch7,
			responseCode: http.StatusOK,
		},
		{
			name:         "success/v8",
			versionResp:  elasticsearch8,
			responseCode: http.StatusOK,
		},
		{
			name:         "client error",
			versionResp:  elasticsearch7,
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to create template: jaeger-template",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				if req.URL.String() == "/" { // ES version check
					res.WriteHeader(http.StatusOK)
					res.Write([]byte(test.versionResp))
					return
				}
				assert.True(t, strings.HasSuffix(req.URL.String(), "_template/jaeger-template"))
				assert.Equal(t, http.MethodPut, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				body, err := io.ReadAll(req.Body)
				assert.NoError(t, err)
				assert.Equal(t, templateContent, string(body))

				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			err := c.CreateTemplate(templateContent, templateName)
			if test.errContains != "" {
				assert.ErrorContains(t, err, test.errContains)
			}
		})
	}
}

func TestRollover(t *testing.T) {
	expectedRequestBody := "{\"conditions\":{\"max_age\":\"2d\"}}"
	mapConditions := map[string]any{
		"max_age": "2d",
	}

	tests := []struct {
		name         string
		responseCode int
		response     string
		errContains  string
	}{
		{
			name:         "success",
			responseCode: http.StatusOK,
		},
		{
			name:         "client error",
			responseCode: http.StatusBadRequest,
			response:     esErrResponse,
			errContains:  "failed to create rollover target: jaeger-span",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.True(t, strings.HasSuffix(req.URL.String(), "jaeger-span/_rollover/"))
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "Basic foobar", req.Header.Get("Authorization"))
				body, err := io.ReadAll(req.Body)
				assert.NoError(t, err)
				assert.Equal(t, expectedRequestBody, string(body))

				res.WriteHeader(test.responseCode)
				res.Write([]byte(test.response))
			}))
			defer testServer.Close()

			c := &IndicesClient{
				Client: Client{
					Client:    testServer.Client(),
					Endpoint:  testServer.URL,
					BasicAuth: "foobar",
				},
			}
			err := c.Rollover("jaeger-span", mapConditions)
			if test.errContains != "" {
				assert.ErrorContains(t, err, test.errContains)
			}
		})
	}
}
