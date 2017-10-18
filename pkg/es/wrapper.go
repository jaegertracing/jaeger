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

// This file avoids lint because the Id and Json are required to be capitalized, but must match an outside library.

// ESClient is a wrapper around elastic.Client
type ESClient struct {
	client *elastic.Client
}

// WrapESClient creates a ESClient out of *elastic.Client.
func WrapESClient(client *elastic.Client) ESClient {
	return ESClient{client: client}
}

// IndexExists calls this function to internal client.
func (c ESClient) IndexExists(index string) IndicesExistsService {
	return WrapESIndicesExistsService(c.client.IndexExists(index))
}

// CreateIndex calls this function to internal client.
func (c ESClient) CreateIndex(index string) IndicesCreateService {
	return WrapESIndicesCreateService(c.client.CreateIndex(index))
}

// Index calls this function to internal client.
func (c ESClient) Index() IndexService {
	return WrapESIndexService(c.client.Index())
}

// Search calls this function to internal client.
func (c ESClient) Search(indices ...string) SearchService {
	return WrapESSearchService(c.client.Search(indices...))
}

// MultiSearch calls this function to internal client.
func (c ESClient) MultiSearch() MultiSearchService {
	return WrapESMultiSearchService(c.client.MultiSearch())
}

// ---

// ESIndicesExistsService is a wrapper around elastic.IndicesExistsService
type ESIndicesExistsService struct {
	indicesExistsService *elastic.IndicesExistsService
}

// WrapESIndicesExistsService creates an ESIndicesExistsService out of *elastic.IndicesExistsService.
func WrapESIndicesExistsService(indicesExistsService *elastic.IndicesExistsService) ESIndicesExistsService {
	return ESIndicesExistsService{indicesExistsService: indicesExistsService}
}

// Do calls this function to internal service.
func (e ESIndicesExistsService) Do(ctx context.Context) (bool, error) {
	return e.indicesExistsService.Do(ctx)
}

// ---

// ESIndicesCreateService is a wrapper around elastic.IndicesCreateService
type ESIndicesCreateService struct {
	indicesCreateService *elastic.IndicesCreateService
}

// WrapESIndicesCreateService creates an ESIndicesCreateService out of *elastic.IndicesCreateService.
func WrapESIndicesCreateService(indicesCreateService *elastic.IndicesCreateService) ESIndicesCreateService {
	return ESIndicesCreateService{indicesCreateService: indicesCreateService}
}

// Body calls this function to internal service.
func (c ESIndicesCreateService) Body(mapping string) IndicesCreateService {
	return WrapESIndicesCreateService(c.indicesCreateService.Body(mapping))
}

// Do calls this function to internal service.
func (c ESIndicesCreateService) Do(ctx context.Context) (*elastic.IndicesCreateResult, error) {
	return c.indicesCreateService.Do(ctx)
}

// ---

// ESIndexService is a wrapper around elastic.ESIndexService
type ESIndexService struct {
	indexService *elastic.IndexService
}

// WrapESIndexService creates an ESIndexService out of *elastic.ESIndexService.
func WrapESIndexService(indexService *elastic.IndexService) ESIndexService {
	return ESIndexService{indexService: indexService}
}

// Index calls this function to internal service.
func (i ESIndexService) Index(index string) IndexService {
	return WrapESIndexService(i.indexService.Index(index))
}

// Type calls this function to internal service.
func (i ESIndexService) Type(typ string) IndexService {
	return WrapESIndexService(i.indexService.Type(typ))
}

// Id calls this function to internal service.
func (i ESIndexService) Id(id string) IndexService {
	return WrapESIndexService(i.indexService.Id(id))
}

// BodyJson calls this function to internal service.
func (i ESIndexService) BodyJson(body interface{}) IndexService {
	return WrapESIndexService(i.indexService.BodyJson(body))
}

// Do calls this function to internal service.
func (i ESIndexService) Do(ctx context.Context) (*elastic.IndexResponse, error) {
	return i.indexService.Do(ctx)
}

// ---

// ESSearchService is a wrapper around elastic.ESSearchService
type ESSearchService struct {
	searchService *elastic.SearchService
}

// WrapESSearchService creates an ESSearchService out of *elastic.ESSearchService.
func WrapESSearchService(searchService *elastic.SearchService) ESSearchService {
	return ESSearchService{searchService: searchService}
}

// Type calls this function to internal service.
func (s ESSearchService) Type(typ string) SearchService {
	return WrapESSearchService(s.searchService.Type(typ))
}

// Size calls this function to internal service.
func (s ESSearchService) Size(size int) SearchService {
	return WrapESSearchService(s.searchService.Size(size))
}

// Aggregation calls this function to internal service.
func (s ESSearchService) Aggregation(name string, aggregation elastic.Aggregation) SearchService {
	return WrapESSearchService(s.searchService.Aggregation(name, aggregation))
}

// IgnoreUnavailable calls this function to internal service.
func (s ESSearchService) IgnoreUnavailable(ignoreUnavailable bool) SearchService {
	return WrapESSearchService(s.searchService.IgnoreUnavailable(ignoreUnavailable))
}

// Query calls this function to internal service.
func (s ESSearchService) Query(query elastic.Query) SearchService {
	return WrapESSearchService(s.searchService.Query(query))
}

// Do calls this function to internal service.
func (s ESSearchService) Do(ctx context.Context) (*elastic.SearchResult, error) {
	return s.searchService.Do(ctx)
}

// ESMultiSearchService is a wrapper around elastic.ESMultiSearchService
type ESMultiSearchService struct {
	multiSearchService *elastic.MultiSearchService
}

// WrapESSearchService creates an ESSearchService out of *elastic.ESSearchService.
func WrapESMultiSearchService(multiSearchService *elastic.MultiSearchService) ESMultiSearchService {
	return ESMultiSearchService{multiSearchService: multiSearchService}
}

// Add calls this function to internal service.
func (s ESMultiSearchService) Add(requests ...*elastic.SearchRequest) MultiSearchService {
	return WrapESMultiSearchService(s.multiSearchService.Add(requests...))
}

// Index calls this function to internal service.
func (s ESMultiSearchService) Index(indices ...string) MultiSearchService {
	return WrapESMultiSearchService(s.multiSearchService.Index(indices...))
}

// Do calls this function to internal service.
func (s ESMultiSearchService) Do(ctx context.Context) (*elastic.MultiSearchResult, error) {
	return s.multiSearchService.Do(ctx)
}
