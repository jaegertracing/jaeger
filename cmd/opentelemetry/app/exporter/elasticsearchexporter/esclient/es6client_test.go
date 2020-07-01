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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

type mockTransport struct {
	Response    *http.Response
	RoundTripFn func(req *http.Request) (*http.Response, error)
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.RoundTripFn(req)
}

func TestES6NewClient_err(t *testing.T) {
	client, err := newElasticsearch6Client(config.Configuration{
		Sniffer: true,
		Servers: []string{"$%"},
	}, &http.Transport{})
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestES6PutTemplateES6Client(t *testing.T) {
	mocktrans := &mockTransport{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(strings.NewReader(`{}`)),
		},
	}
	mocktrans.RoundTripFn = func(req *http.Request) (*http.Response, error) { return mocktrans.Response, nil }
	client, err := newElasticsearch6Client(config.Configuration{}, mocktrans)
	require.NoError(t, err)
	assert.NotNil(t, client)
	err = client.PutTemplate("foo", strings.NewReader("bar"))
	require.NoError(t, err)
}

func TestES6AddDataToBulk(t *testing.T) {
	client, err := newElasticsearch6Client(config.Configuration{}, &http.Transport{})
	require.NoError(t, err)
	assert.NotNil(t, client)

	buf := &bytes.Buffer{}
	client.AddDataToBulkBuffer(buf, []byte("data"), "foo", "bar")
	assert.Equal(t, "{\"index\":{\"_index\":\"foo\",\"_type\":\"bar\"}}\ndata\n", buf.String())
}

func TestES6Bulk(t *testing.T) {
	tests := []struct {
		resp *http.Response
		err  string
	}{
		{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader("{}")),
			},
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
			mocktrans.RoundTripFn = func(req *http.Request) (*http.Response, error) { return mocktrans.Response, nil }

			client, err := newElasticsearch6Client(config.Configuration{}, mocktrans)
			require.NoError(t, err)
			assert.NotNil(t, client)
			_, err = client.Bulk(strings.NewReader("data"))
			if test.err != "" {
				fmt.Println()
				assert.Contains(t, err.Error(), test.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
