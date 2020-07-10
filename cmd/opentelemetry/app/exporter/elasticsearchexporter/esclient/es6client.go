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
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	elasticsearch6 "github.com/elastic/go-elasticsearch/v6"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

const (
	bulkES6MetaFormat = `{"index":{"_index":"%s","_type":"%s"}}` + "\n"
)

type elasticsearch6Client struct {
	client *elasticsearch6.Client
}

var _ ElasticsearchClient = (*elasticsearch6Client)(nil)

func newElasticsearch6Client(params config.Configuration, roundTripper http.RoundTripper) (*elasticsearch6Client, error) {
	client, err := elasticsearch6.NewClient(elasticsearch6.Config{
		DiscoverNodesOnStart: params.Sniffer,
		Addresses:            params.Servers,
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

func (es *elasticsearch6Client) PutTemplate(name string, body io.Reader) error {
	resp, err := es.client.Indices.PutTemplate(name, body)
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

func (es *elasticsearch6Client) Bulk(reader io.Reader) (*BulkResponse, error) {
	response, err := es.client.Bulk(reader)
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
