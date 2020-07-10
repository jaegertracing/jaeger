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
	"io"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

// ElasticsearchClient exposes Elasticsearch API used by Jaeger
type ElasticsearchClient interface {
	// PutTemplate creates index template
	PutTemplate(name string, template io.Reader) error
	// Bulk submits a bulk request
	Bulk(bulkBody io.Reader) (*BulkResponse, error)
	// AddDataToBulkBuffer creates bulk item from data, index and typ and adds it to bulkBody
	AddDataToBulkBuffer(bulkBody *bytes.Buffer, data []byte, index, typ string)
}

// BulkResponse is a response returned by Elasticsearch Bulk API
type BulkResponse struct {
	Errors bool `json:"errors"`
	Items  []struct {
		Index struct {
			ID     string `json:"_id"`
			Result string `json:"result"`
			Status int    `json:"status"`
			Error  struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
				Cause  struct {
					Type   string `json:"type"`
					Reason string `json:"reason"`
				} `json:"caused_by"`
			} `json:"error"`
		} `json:"index"`
	} `json:"items"`
}

// NewElasticsearchClient returns an instance of Elasticsearch client
func NewElasticsearchClient(params config.Configuration, logger *zap.Logger) (ElasticsearchClient, error) {
	roundTripper, err := config.GetHTTPRoundTripper(&params, logger)
	if err != nil {
		return nil, err
	}
	if params.GetVersion() == 0 {
		esPing := elasticsearchPing{
			username:     params.Username,
			password:     params.Password,
			roundTripper: roundTripper,
		}
		esVersion, err := esPing.getVersion(params.Servers[0])
		if err != nil {
			return nil, err
		}
		logger.Info("Elasticsearch detected", zap.Int("version", esVersion))
		params.Version = uint(esVersion)
	}
	switch params.Version {
	case 5, 6:
		return newElasticsearch6Client(params, roundTripper)
	case 7:
		return newElasticsearch7Client(params, roundTripper)
	default:
		return nil, fmt.Errorf("could not create Elasticseach client for version %d", params.Version)
	}
}
