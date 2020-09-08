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
	"net/http"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es/config"
)

var errMissingURL = fmt.Errorf("missing Elasticsearch URL")

// ElasticsearchClient exposes Elasticsearch API used by Jaeger.
// This is not a general purpose ES client implementation.
// The exposed APIs are the bare minimum that is used by Jaeger project to store and query data.
type ElasticsearchClient interface {
	// PutTemplate creates index template
	PutTemplate(ctx context.Context, name string, template io.Reader) error
	// Bulk submits a bulk request
	Bulk(ctx context.Context, bulkBody io.Reader) (*BulkResponse, error)
	// AddDataToBulkBuffer creates bulk item from data, index and typ and adds it to bulkBody
	AddDataToBulkBuffer(bulkBody *bytes.Buffer, data []byte, index, typ string)
	// Index indexes data into storage
	Index(ctx context.Context, body io.Reader, index, typ string) error

	// Search searches data via /_search
	Search(ctx context.Context, query SearchBody, size int, indices ...string) (*SearchResponse, error)
	// MultiSearch searches data via /_msearch
	MultiSearch(ctx context.Context, queries []SearchBody) (*MultiSearchResponse, error)

	// Major version returns major ES version
	MajorVersion() int
}

// BulkResponse is a response returned by Elasticsearch Bulk API
type BulkResponse struct {
	Errors bool               `json:"errors"`
	Items  []BulkResponseItem `json:"items"`
}

// BulkResponseItem is a single response from BulkResponse
type BulkResponseItem struct {
	Index BulkIndexResponse `json:"index"`
}

// BulkIndexResponse is a bulk response for index action
type BulkIndexResponse struct {
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
}

// SearchBody defines search request.
type SearchBody struct {
	// indices are not in body, the ES client puts them to request path
	Indices        []string           `json:"-"`
	Aggregations   json.RawMessage    `json:"aggs,omitempty"`
	Query          *Query             `json:"query,omitempty"`
	Sort           []map[string]Order `json:"sort,omitempty"`
	Size           int                `json:"size"`
	TerminateAfter int                `json:"terminate_after"`
	SearchAfter    []interface{}      `json:"search_after,omitempty"`
}

// Order defines order in the query.
type Order string

const (
	// AscOrder defines ascending order.
	AscOrder Order = "asc"
)

// BoolQueryType defines bool query type.
type BoolQueryType string

// Must defines must bool query type.
const Must BoolQueryType = "must"

// Should defines should bool query type.
const Should BoolQueryType = "should"

// Query defines search query.
type Query struct {
	Term         *Terms                        `json:"term,omitempty"`
	RangeQueries map[string]RangeQuery         `json:"range,omitempty"`
	BoolQuery    map[BoolQueryType][]BoolQuery `json:"bool,omitempty"`
}

// BoolQuery defines bool query.
type BoolQuery struct {
	Term         map[string]string             `json:"term,omitempty"`
	Regexp       map[string]TermQuery          `json:"regexp,omitempty"`
	Nested       *NestedQuery                  `json:"nested,omitempty"`
	BoolQuery    map[BoolQueryType][]BoolQuery `json:"bool,omitempty"`
	RangeQueries map[string]RangeQuery         `json:"range,omitempty"`
	MatchQueries map[string]MatchQuery         `json:"match,omitempty"`
}

// NestedQuery defines nested query.
type NestedQuery struct {
	Path  string `json:"path"`
	Query Query  `json:"query"`
}

// RangeQuery defines range query.
type RangeQuery struct {
	GTE interface{} `json:"gte"`
	LTE interface{} `json:"lte"`
}

// Terms defines terms query.
type Terms map[string]TermQuery

// TermQuery defines term query.
type TermQuery struct {
	Value string `json:"value"`
}

// MatchQuery defines match query.
type MatchQuery struct {
	Query string `json:"query"`
}

// MultiSearchResponse defines multi search response.
type MultiSearchResponse struct {
	Responses []SearchResponse `json:"responses"`
}

// SearchResponse defines search response.
type SearchResponse struct {
	Hits  Hits                           `json:"hits"`
	Aggs  map[string]AggregationResponse `json:"aggregations,omitempty"`
	Error *SearchResponseError           `json:"error,omitempty"`
}

// SearchResponseError defines search response error.
type SearchResponseError struct {
	json.RawMessage
}

var _ fmt.Stringer = (*SearchResponseError)(nil)

func (e *SearchResponseError) String() string {
	if e.RawMessage == nil {
		return ""
	}
	return string(e.RawMessage)
}

// Hits defines search hits.
type Hits struct {
	Total int   `json:"total"`
	Hits  []Hit `json:"hits"`
}

// Hit defines a single search hit.
type Hit struct {
	Source *json.RawMessage `json:"_source"`
}

// AggregationResponse defines aggregation response.
type AggregationResponse struct {
	Buckets []struct {
		Key string `json:"key"`
	} `json:"buckets"`
}

// NewElasticsearchClient returns an instance of Elasticsearch client
func NewElasticsearchClient(params config.Configuration, logger *zap.Logger) (ElasticsearchClient, error) {
	if len(params.Servers) == 0 {
		return nil, errMissingURL
	}

	roundTripper, err := config.GetHTTPRoundTripper(&params, logger)
	if err != nil {
		return nil, err
	}
	esVersion := int(params.GetVersion())
	if esVersion == 0 {
		esPing := elasticsearchPing{
			username:     params.Username,
			password:     params.Password,
			roundTripper: roundTripper,
		}
		esVersion, err = esPing.getVersion(params.Servers[0])
		if err != nil {
			return nil, err
		}
		logger.Info("Elasticsearch detected", zap.Int("version", esVersion))
	}
	return newElasticsearchClient(esVersion, clientConfig{
		DiscoverNotesOnStartup: params.Sniffer,
		Addresses:              params.Servers,
		Username:               params.Username,
		Password:               params.Password,
	}, roundTripper)
}

type clientConfig struct {
	DiscoverNotesOnStartup bool
	Addresses              []string
	Username               string
	Password               string
}

func newElasticsearchClient(version int, params clientConfig, roundTripper http.RoundTripper) (ElasticsearchClient, error) {
	switch version {
	case 5, 6:
		return newElasticsearch6Client(params, roundTripper)
	case 7:
		return newElasticsearch7Client(params, roundTripper)
	default:
		return nil, fmt.Errorf("could not create Elasticseach client for version %d", version)
	}
}
