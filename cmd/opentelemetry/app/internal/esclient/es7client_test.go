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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestES7NewClient_err(t *testing.T) {
	client, err := newElasticsearch7Client(clientConfig{
		Addresses: []string{"$%"},
	}, &http.Transport{})
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestES7PutTemplate(t *testing.T) {
	testPutTemplate(t, func(tripper http.RoundTripper) (ElasticsearchClient, error) {
		return newElasticsearch7Client(clientConfig{}, tripper)
	})
}

func TestES7AddDataToBulk(t *testing.T) {
	client, err := newElasticsearch7Client(clientConfig{}, &http.Transport{})
	require.NoError(t, err)
	assert.NotNil(t, client)

	buf := &bytes.Buffer{}
	client.AddDataToBulkBuffer(buf, []byte("data"), "foo", "bar")
	assert.Equal(t, "{\"index\":{\"_index\":\"foo\"}}\ndata\n", buf.String())
}

func TestES7Bulk(t *testing.T) {
	testBulk(t, func(tripper http.RoundTripper) (ElasticsearchClient, error) {
		return newElasticsearch7Client(clientConfig{}, tripper)
	})
}

func TestES7Index(t *testing.T) {
	testIndex(t, func(tripper http.RoundTripper) (ElasticsearchClient, error) {
		return newElasticsearch7Client(clientConfig{}, tripper)
	})
}

func TestES7Search(t *testing.T) {
	testSearch(t, func(tripper http.RoundTripper) (ElasticsearchClient, error) {
		return newElasticsearch7Client(clientConfig{}, tripper)
	})
}

func TestES7MultiSearch(t *testing.T) {
	testMultiSearch(t, func(tripper http.RoundTripper) (ElasticsearchClient, error) {
		return newElasticsearch7Client(clientConfig{}, tripper)
	})
}

func TestES7Version(t *testing.T) {
	c, err := newElasticsearch7Client(clientConfig{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 7, c.MajorVersion())
}
