// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package es

import (
	"context"

	"github.com/olivere/elastic"
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
