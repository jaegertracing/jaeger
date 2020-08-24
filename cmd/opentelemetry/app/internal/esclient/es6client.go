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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	elasticsearch6 "github.com/elastic/go-elasticsearch/v6"
)

const (
	bulkES6MetaFormat = `{"index":{"_index":"%s","_type":"%s"}}` + "\n"
)

type elasticsearch6Client struct {
	client *elasticsearch6.Client
}

var _ ElasticsearchClient = (*elasticsearch6Client)(nil)

func newElasticsearch6Client(params clientConfig, roundTripper http.RoundTripper) (*elasticsearch6Client, error) {
	client, err := elasticsearch6.NewClient(elasticsearch6.Config{
		DiscoverNodesOnStart: params.DiscoverNotesOnStartup,
		Addresses:            params.Addresses,
		Username:             params.Username,
		Password:             params.Password,
		Transport:            roundTripper,
	})
	if err != nil {
		return nil, err
	}
	return &elasticsearch6Client{
		client: client,
	}, nil
}

func (es *elasticsearch6Client) PutTemplate(ctx context.Context, name string, body io.Reader) error {
	resp, err := es.client.Indices.PutTemplate(name, body, es.client.Indices.PutTemplate.WithContext(ctx))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (es *elasticsearch6Client) AddDataToBulkBuffer(buffer *bytes.Buffer, data []byte, index, typ string) {
	meta := []byte(fmt.Sprintf(bulkES6MetaFormat, index, typ))
	buffer.Grow(len(meta) + len(data) + len("\n"))
	buffer.Write(meta)
	buffer.Write(data)
	buffer.Write([]byte("\n"))
}

func (es *elasticsearch6Client) Bulk(ctx context.Context, reader io.Reader) (*BulkResponse, error) {
	response, err := es.client.Bulk(reader, es.client.Bulk.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("bulk request failed with code %d", response.StatusCode)
	}
	var blk BulkResponse
	err = json.NewDecoder(response.Body).Decode(&blk)
	if err != nil {
		return nil, err
	}
	return &blk, nil
}

func (es *elasticsearch6Client) Index(ctx context.Context, body io.Reader, index, typ string) error {
	response, err := es.client.Index(index, body, es.client.Index.WithContext(ctx), es.client.Index.WithDocumentType(typ))
	if err != nil {
		return err
	}
	return response.Body.Close()
}

func (es *elasticsearch6Client) Search(ctx context.Context, query SearchBody, size int, indices ...string) (*SearchResponse, error) {
	body, err := encodeSearchBody(query)
	if err != nil {
		return nil, err
	}

	response, err := es.client.Search(
		es.client.Search.WithContext(ctx),
		es.client.Search.WithIndex(indices...),
		es.client.Search.WithBody(body),
		es.client.Search.WithIgnoreUnavailable(true),
		es.client.Search.WithSize(size))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	r := &SearchResponse{}
	if err = json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (es *elasticsearch6Client) MultiSearch(ctx context.Context, queries []SearchBody) (*MultiSearchResponse, error) {
	body, err := encodeSearchBodies(queries)
	if err != nil {
		return nil, err
	}

	var indices []string
	for _, q := range queries {
		indices = append(indices, q.Indices...)
	}

	response, err := es.client.Msearch(body,
		es.client.Msearch.WithContext(ctx),
		es.client.Msearch.WithIndex(indices...),
	)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	r := &MultiSearchResponse{}
	if err = json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (es *elasticsearch6Client) MajorVersion() int {
	return 6
}
