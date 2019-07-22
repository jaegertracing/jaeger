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

package eswrapper

import (
	"context"

	"github.com/olivere/elastic"

	"github.com/jaegertracing/jaeger/pkg/es"
)

// This file avoids lint because the Id and Json are required to be capitalized, but must match an outside library.

// ClientWrapper is a wrapper around elastic.Client
type ClientWrapper struct {
	client      *elastic.Client
	bulkService *elastic.BulkProcessor
}

// WrapESClient creates a ESClient out of *elastic.Client.
func WrapESClient(client *elastic.Client, s *elastic.BulkProcessor) ClientWrapper {
	return ClientWrapper{client: client, bulkService: s}
}

// IndexExists calls this function to internal client.
func (c ClientWrapper) IndexExists(index string) es.IndicesExistsService {
	return WrapESIndicesExistsService(c.client.IndexExists(index))
}

// CreateIndex calls this function to internal client.
func (c ClientWrapper) CreateIndex(index string) es.IndicesCreateService {
	return WrapESIndicesCreateService(c.client.CreateIndex(index))
}

// CreateTemplate calls this function to internal client.
func (c ClientWrapper) CreateTemplate(ttype string) es.TemplateCreateService {
	return WrapESTemplateCreateService(c.client.IndexPutTemplate(ttype))
}

// Index calls this function to internal client.
func (c ClientWrapper) Index() es.IndexService {
	r := elastic.NewBulkIndexRequest()
	return WrapESIndexService(r, c.bulkService)
}

// Search calls this function to internal client.
func (c ClientWrapper) Search(indices ...string) es.SearchService {
	return WrapESSearchService(c.client.Search(indices...))
}

// MultiSearch calls this function to internal client.
func (c ClientWrapper) MultiSearch() es.MultiSearchService {
	return WrapESMultiSearchService(c.client.MultiSearch())
}

// Close closes ESClient and flushes all data to the storage.
func (c ClientWrapper) Close() error {
	return c.bulkService.Close()
}

// ---

// IndicesExistsServiceWrapper is a wrapper around elastic.IndicesExistsService
type IndicesExistsServiceWrapper struct {
	indicesExistsService *elastic.IndicesExistsService
}

// WrapESIndicesExistsService creates an ESIndicesExistsService out of *elastic.IndicesExistsService.
func WrapESIndicesExistsService(indicesExistsService *elastic.IndicesExistsService) IndicesExistsServiceWrapper {
	return IndicesExistsServiceWrapper{indicesExistsService: indicesExistsService}
}

// Do calls this function to internal service.
func (e IndicesExistsServiceWrapper) Do(ctx context.Context) (bool, error) {
	return e.indicesExistsService.Do(ctx)
}

// ---

// IndicesCreateServiceWrapper is a wrapper around elastic.IndicesCreateService
type IndicesCreateServiceWrapper struct {
	indicesCreateService *elastic.IndicesCreateService
}

// WrapESIndicesCreateService creates an ESIndicesCreateService out of *elastic.IndicesCreateService.
func WrapESIndicesCreateService(indicesCreateService *elastic.IndicesCreateService) IndicesCreateServiceWrapper {
	return IndicesCreateServiceWrapper{indicesCreateService: indicesCreateService}
}

// Body calls this function to internal service.
func (c IndicesCreateServiceWrapper) Body(mapping string) es.IndicesCreateService {
	return WrapESIndicesCreateService(c.indicesCreateService.Body(mapping))
}

// IncludeTypeName calls this function to internal service.
func (c IndicesCreateServiceWrapper) IncludeTypeName(include bool) es.IndicesCreateService {
	return WrapESIndicesCreateService(c.indicesCreateService.IncludeTypeName(include))
}

// Do calls this function to internal service.
func (c IndicesCreateServiceWrapper) Do(ctx context.Context) (*elastic.IndicesCreateResult, error) {
	return c.indicesCreateService.Do(ctx)
}

// TemplateCreateServiceWrapper is a wrapper around elastic.IndicesPutTemplateService.
type TemplateCreateServiceWrapper struct {
	mappingCreateService *elastic.IndicesPutTemplateService
}

// WrapESTemplateCreateService creates an TemplateCreateService out of *elastic.IndicesPutTemplateService.
func WrapESTemplateCreateService(mappingCreateService *elastic.IndicesPutTemplateService) TemplateCreateServiceWrapper {
	return TemplateCreateServiceWrapper{mappingCreateService: mappingCreateService}
}

// Body calls this function to internal service.
func (c TemplateCreateServiceWrapper) Body(mapping string) es.TemplateCreateService {
	return WrapESTemplateCreateService(c.mappingCreateService.BodyString(mapping))
}

// Do calls this function to internal service.
func (c TemplateCreateServiceWrapper) Do(ctx context.Context) (*elastic.IndicesPutTemplateResponse, error) {
	return c.mappingCreateService.Do(ctx)
}

// ---

// IndexServiceWrapper is a wrapper around elastic.ESIndexService.
// See wrapper_nolint.go for more functions.
type IndexServiceWrapper struct {
	bulkIndexReq *elastic.BulkIndexRequest
	bulkService  *elastic.BulkProcessor
}

// WrapESIndexService creates an ESIndexService out of *elastic.ESIndexService.
func WrapESIndexService(indexService *elastic.BulkIndexRequest, bulkService *elastic.BulkProcessor) IndexServiceWrapper {
	return IndexServiceWrapper{bulkIndexReq: indexService, bulkService: bulkService}
}

// Index calls this function to internal service.
func (i IndexServiceWrapper) Index(index string) es.IndexService {
	return WrapESIndexService(i.bulkIndexReq.Index(index), i.bulkService)
}

// Type calls this function to internal service.
func (i IndexServiceWrapper) Type(typ string) es.IndexService {
	return WrapESIndexService(i.bulkIndexReq.Type(typ), i.bulkService)
}

// Add adds the request to bulk service
func (i IndexServiceWrapper) Add() {
	i.bulkService.Add(i.bulkIndexReq)
}

// ---

// SearchServiceWrapper is a wrapper around elastic.ESSearchService
type SearchServiceWrapper struct {
	searchService *elastic.SearchService
}

// WrapESSearchService creates an ESSearchService out of *elastic.ESSearchService.
func WrapESSearchService(searchService *elastic.SearchService) SearchServiceWrapper {
	return SearchServiceWrapper{searchService: searchService}
}

// Type calls this function to internal service.
func (s SearchServiceWrapper) Type(typ string) es.SearchService {
	return WrapESSearchService(s.searchService.Type(typ))
}

// Size calls this function to internal service.
func (s SearchServiceWrapper) Size(size int) es.SearchService {
	return WrapESSearchService(s.searchService.Size(size))
}

// Aggregation calls this function to internal service.
func (s SearchServiceWrapper) Aggregation(name string, aggregation elastic.Aggregation) es.SearchService {
	return WrapESSearchService(s.searchService.Aggregation(name, aggregation))
}

// IgnoreUnavailable calls this function to internal service.
func (s SearchServiceWrapper) IgnoreUnavailable(ignoreUnavailable bool) es.SearchService {
	return WrapESSearchService(s.searchService.IgnoreUnavailable(ignoreUnavailable))
}

// Query calls this function to internal service.
func (s SearchServiceWrapper) Query(query elastic.Query) es.SearchService {
	return WrapESSearchService(s.searchService.Query(query))
}

// Do calls this function to internal service.
func (s SearchServiceWrapper) Do(ctx context.Context) (*elastic.SearchResult, error) {
	return s.searchService.Do(ctx)
}

// MultiSearchServiceWrapper is a wrapper around elastic.ESMultiSearchService
type MultiSearchServiceWrapper struct {
	multiSearchService *elastic.MultiSearchService
}

// WrapESMultiSearchService creates an ESSearchService out of *elastic.ESSearchService.
func WrapESMultiSearchService(multiSearchService *elastic.MultiSearchService) MultiSearchServiceWrapper {
	return MultiSearchServiceWrapper{multiSearchService: multiSearchService}
}

// Add calls this function to internal service.
func (s MultiSearchServiceWrapper) Add(requests ...*elastic.SearchRequest) es.MultiSearchService {
	return WrapESMultiSearchService(s.multiSearchService.Add(requests...))
}

// Index calls this function to internal service.
func (s MultiSearchServiceWrapper) Index(indices ...string) es.MultiSearchService {
	return WrapESMultiSearchService(s.multiSearchService.Index(indices...))
}

// Do calls this function to internal service.
func (s MultiSearchServiceWrapper) Do(ctx context.Context) (*elastic.MultiSearchResult, error) {
	return s.multiSearchService.Do(ctx)
}
