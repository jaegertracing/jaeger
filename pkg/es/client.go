// Copyright (c) 2017 Uber Technologies, Inc.
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

package es

import (
	"context"

	"gopkg.in/olivere/elastic.v5"
)

// Client is an abstraction for elastic.Client
type Client interface {
	IndexExists(index string) IndicesExistsService
	CreateIndex(index string) IndicesCreateService
	Index() IndexService
	Search(indices ...string) SearchService
	MultiSearch() MultiSearchService
}

// IndicesExistsService is an abstraction for elastic.IndicesExistsService
type IndicesExistsService interface {
	Do(ctx context.Context) (bool, error)
}

// IndicesCreateService is an abstraction for elastic.IndicesCreateService
type IndicesCreateService interface {
	Body(mapping string) IndicesCreateService
	Do(ctx context.Context) (*elastic.IndicesCreateResult, error)
}

// IndexService is an abstraction for elastic.IndexService
type IndexService interface {
	Index(index string) IndexService
	Type(typ string) IndexService
	Id(id string) IndexService
	BodyJson(body interface{}) IndexService
	Do(ctx context.Context) (*elastic.IndexResponse, error)
}

// SearchService is an abstraction for elastic.SearchService
type SearchService interface {
	Type(typ string) SearchService
	Size(size int) SearchService
	Aggregation(name string, aggregation elastic.Aggregation) SearchService
	IgnoreUnavailable(ignoreUnavailable bool) SearchService
	Query(query elastic.Query) SearchService
	Do(ctx context.Context) (*elastic.SearchResult, error)
}

// MultiSearchService is an abstraction for elastic.MultiSearchService
type MultiSearchService interface {
	Add(requests ...*elastic.SearchRequest) MultiSearchService
	Index(indices ...string) MultiSearchService
	Do(ctx context.Context) (*elastic.MultiSearchResult, error)
}
