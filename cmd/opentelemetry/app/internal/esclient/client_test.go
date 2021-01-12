// Copyright (c) 2020 The Jaeger Authors.
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

package esclient

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

func TestGetClient(t *testing.T) {
	tests := []struct {
		version int
		err     string
	}{
		{
			version: 5,
		},
		{
			version: 6,
		},
		{
			version: 7,
		},
		{
			version: 1,
			err:     "could not create Elasticseach client for version 1",
		},
	}
	for _, test := range tests {
		t.Run(test.err, func(t *testing.T) {
			client, err := NewElasticsearchClient(config.Configuration{Servers: []string{""}, Version: uint(test.version)}, zap.NewNop())
			if test.err == "" {
				require.NoError(t, err)
			}
			switch test.version {
			case 5, 6:
				assert.IsType(t, &elasticsearch6Client{}, client)
			case 7:
				assert.IsType(t, &elasticsearch7Client{}, client)
			default:
				assert.EqualError(t, err, test.err)
				assert.Nil(t, client)
			}
		})
	}
}

type mockTransport struct {
	Response *http.Response
	Err      error
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.Response, t.Err
}

type mockReader struct {
	err error
}

var _ io.Reader = (*mockReader)(nil)

func (m mockReader) Read(p []byte) (n int, err error) {
	return 0, m.err
}

func testPutTemplate(t *testing.T, clientFactory func(tripper http.RoundTripper) (ElasticsearchClient, error)) {
	tests := []struct {
		name         string
		transportErr error
	}{
		{
			name:         "body reader error",
			transportErr: fmt.Errorf("failed to get body"),
		},
		{
			name: "success",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mocktrans := &mockTransport{
				Response: &http.Response{
					StatusCode: http.StatusOK,
					Body:       ioutil.NopCloser(strings.NewReader("{}")),
				},
				Err: test.transportErr,
			}
			client, err := clientFactory(mocktrans)
			require.NoError(t, err)
			assert.NotNil(t, client)

			err = client.PutTemplate(context.Background(), "foo", strings.NewReader(""))
			if test.transportErr != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.transportErr.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func testBulk(t *testing.T, clientFactory func(tripper http.RoundTripper) (ElasticsearchClient, error)) {
	tests := []struct {
		expectedResponse *BulkResponse
		resp             *http.Response
		err              string
	}{
		{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader("{}")),
			},
			expectedResponse: &BulkResponse{},
		},
		{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader("{#}")),
			},
			err: "looking for beginning of object key string",
		},
		{
			resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       ioutil.NopCloser(strings.NewReader("{#}")),
			},
			err: "bulk request failed with code 400",
		},
	}
	for _, test := range tests {
		t.Run(test.err, func(t *testing.T) {
			mocktrans := &mockTransport{
				Response: test.resp,
			}
			client, err := clientFactory(mocktrans)
			require.NoError(t, err)
			assert.NotNil(t, client)

			bulkResp, err := client.Bulk(context.Background(), strings.NewReader("data"))
			if test.err != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expectedResponse, bulkResp)
		})
	}
}

func testIndex(t *testing.T, clientFactory func(tripper http.RoundTripper) (ElasticsearchClient, error)) {
	tests := []struct {
		err error
	}{
		{},
		{
			err: fmt.Errorf("wrong request"),
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v", test.err), func(t *testing.T) {
			mocktrans := &mockTransport{
				Response: &http.Response{
					StatusCode: 200,
					Body:       ioutil.NopCloser(strings.NewReader("")),
				},
				Err: test.err,
			}
			client, err := clientFactory(mocktrans)
			require.NoError(t, err)
			require.NotNil(t, client)

			err = client.Index(context.Background(), strings.NewReader(""), "", "")
			if test.err != nil {
				assert.EqualError(t, err, test.err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func testSearch(t *testing.T, clientFactory func(tripper http.RoundTripper) (ElasticsearchClient, error)) {
	tests := []struct {
		name             string
		searchBody       SearchBody
		transportError   error
		response         *http.Response
		expectedError    string
		expectedResponse *SearchResponse
	}{
		{
			name:          "marshall query error",
			searchBody:    SearchBody{SearchAfter: []interface{}{make(chan bool)}},
			expectedError: "unsupported type",
		},
		{
			name:           "transport error",
			transportError: fmt.Errorf("transport err"),
			expectedError:  "transport err",
		},
		{
			name: "read response body error",
			response: &http.Response{
				Body: ioutil.NopCloser(mockReader{err: fmt.Errorf("failed to read body")}),
			},
			expectedError: "failed to read body",
		},
		{
			name: "unmarshall body error",
			response: &http.Response{
				Body: ioutil.NopCloser(strings.NewReader("@")),
			},
			expectedError: "invalid character '@'",
		},
		{
			name: "success",
			response: &http.Response{
				Body: ioutil.NopCloser(strings.NewReader("{}")),
			},
			expectedResponse: &SearchResponse{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mocktrans := &mockTransport{
				Response: test.response,
				Err:      test.transportError,
			}
			client, err := clientFactory(mocktrans)
			require.NoError(t, err)
			require.NotNil(t, client)

			response, err := client.Search(context.Background(), test.searchBody, 0)
			if test.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				assert.Nil(t, response)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expectedResponse, response)
		})
	}
}

func testMultiSearch(t *testing.T, clientFactory func(tripper http.RoundTripper) (ElasticsearchClient, error)) {
	tests := []struct {
		name             string
		searchBody       SearchBody
		transportError   error
		response         *http.Response
		expectedError    string
		expectedResponse *MultiSearchResponse
	}{
		{
			name:          "marshall query error",
			searchBody:    SearchBody{SearchAfter: []interface{}{make(chan bool)}},
			expectedError: "unsupported type",
		},
		{
			name:           "transport error",
			transportError: fmt.Errorf("transport err"),
			expectedError:  "transport err",
		},
		{
			name: "read response body error",
			response: &http.Response{
				Body: ioutil.NopCloser(mockReader{err: fmt.Errorf("failed to read body")}),
			},
			expectedError: "failed to read body",
		},
		{
			name: "unmarshall body error",
			response: &http.Response{
				Body: ioutil.NopCloser(strings.NewReader("@")),
			},
			expectedError: "invalid character '@'",
		},
		{
			name: "success",
			response: &http.Response{
				Body: ioutil.NopCloser(strings.NewReader("{\"responses\": [{}, {}]}")),
			},
			expectedResponse: &MultiSearchResponse{Responses: []SearchResponse{{}, {}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mocktrans := &mockTransport{
				Response: test.response,
				Err:      test.transportError,
			}
			client, err := clientFactory(mocktrans)
			require.NoError(t, err)
			require.NotNil(t, client)

			response, err := client.MultiSearch(context.Background(), []SearchBody{test.searchBody})
			if test.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				assert.Nil(t, response)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expectedResponse, response)
		})
	}
}

func TestGetClient_err_missingURL(t *testing.T) {
	client, err := NewElasticsearchClient(config.Configuration{}, zap.NewNop())
	require.Error(t, err)
	assert.EqualError(t, err, errMissingURL.Error())
	assert.Nil(t, client)
}
