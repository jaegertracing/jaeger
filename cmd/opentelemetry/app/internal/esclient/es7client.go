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

	elasticsearch7 "github.com/elastic/go-elasticsearch/v7"
)

const (
	bulkES7MetaFormat = `{"index":{"_index":"%s"}}` + "\n"
)

type elasticsearch7Client struct {
	client *elasticsearch7.Client
}

var _ ElasticsearchClient = (*elasticsearch7Client)(nil)

func newElasticsearch7Client(config clientConfig, roundTripper http.RoundTripper) (*elasticsearch7Client, error) {
	client, err := elasticsearch7.NewClient(elasticsearch7.Config{
		Addresses: config.Addresses,
		Username:  config.Username,
		Password:  config.Password,
		Transport: roundTripper,
	})
	if err != nil {
		return nil, err
	}
	return &elasticsearch7Client{
		client: client,
	}, nil
}

func (es *elasticsearch7Client) PutTemplate(ctx context.Context, name string, body io.Reader) error {
	resp, err := es.client.Indices.PutTemplate(body, name, es.client.Indices.PutTemplate.WithContext(ctx))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (es *elasticsearch7Client) AddDataToBulkBuffer(buffer *bytes.Buffer, data []byte, index, _ string) {
	meta := []byte(fmt.Sprintf(bulkES7MetaFormat, index))
	buffer.Grow(len(meta) + len(data) + len("\n"))
	buffer.Write(meta)
	buffer.Write(data)
	buffer.Write([]byte("\n"))
}

func (es *elasticsearch7Client) Bulk(ctx context.Context, reader io.Reader) (*BulkResponse, error) {
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

func (es *elasticsearch7Client) Index(ctx context.Context, body io.Reader, index string, _ string) error {
	response, err := es.client.Index(index, body, es.client.Index.WithContext(ctx))
	if err != nil {
		return err
	}
	return response.Body.Close()
}

func (es *elasticsearch7Client) Search(ctx context.Context, query SearchBody, size int, indices ...string) (*SearchResponse, error) {
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
	r := &es7searchResponse{}
	if err = json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	convertedResp := convertSearchResponse(*r)
	return &convertedResp, nil
}

func (es *elasticsearch7Client) MultiSearch(ctx context.Context, queries []SearchBody) (*MultiSearchResponse, error) {
	var indices []string
	var es7Queries []es7SearchBody
	for _, q := range queries {
		es7Queries = append(es7Queries, es7SearchBody{
			SearchBody:     q,
			TrackTotalHits: true,
		})
		indices = append(indices, q.Indices...)
	}
	body, err := es7QueryBodies(es7Queries)
	if err != nil {
		return nil, err
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
	r := &es7multiSearchResponse{}
	if err = json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	return convertMultiSearchResponse(r), nil
}

func (es *elasticsearch7Client) MajorVersion() int {
	return 7
}

func convertMultiSearchResponse(response *es7multiSearchResponse) *MultiSearchResponse {
	mResponse := &MultiSearchResponse{}
	for _, r := range response.Responses {
		mResponse.Responses = append(mResponse.Responses, convertSearchResponse(r))
	}
	return mResponse
}

func convertSearchResponse(response es7searchResponse) SearchResponse {
	return SearchResponse{
		Aggs: response.Aggs,
		Hits: Hits{
			Total: response.Hits.Total.Value,
			Hits:  response.Hits.Hits,
		},
	}
}

// Override the SearchBody to add TrackTotalHits compatible with ES7
type es7SearchBody struct {
	SearchBody
	TrackTotalHits bool `json:"track_total_hits"`
}

type es7multiSearchResponse struct {
	Responses []es7searchResponse `json:"responses"`
}

type es7searchResponse struct {
	Hits es7its                         `json:"hits"`
	Aggs map[string]AggregationResponse `json:"aggregations,omitempty"`
}

type es7its struct {
	Total struct {
		Value int `json:"value"`
	} `json:"total"`
	Hits []Hit `json:"hits"`
}

func es7QueryBodies(searchBodies []es7SearchBody) (io.Reader, error) {
	buf := &bytes.Buffer{}
	for _, sb := range searchBodies {
		data, err := json.Marshal(sb)
		if err != nil {
			return nil, err
		}
		addDataToMSearchBuffer(buf, data)
	}
	return buf, nil
}
