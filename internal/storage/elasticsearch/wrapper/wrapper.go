// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package eswrapper

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	esv8 "github.com/elastic/go-elasticsearch/v9"
	esv8api "github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/olivere/elastic/v7"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

// This file avoids lint because the Id and Json are required to be capitalized, but must match an outside library.

// ClientWrapper is a wrapper around elastic.Client
type ClientWrapper struct {
	client      *elastic.Client
	bulkService *elastic.BulkProcessor
	version     es.BackendVersion
	clientV8    *esv8.Client
}

// GetVersion returns the backend version.
func (c ClientWrapper) GetVersion() es.BackendVersion {
	return c.version
}

// WrapESClient creates a ESClient out of *elastic.Client.
func WrapESClient(client *elastic.Client, s *elastic.BulkProcessor, version es.BackendVersion, clientV8 *esv8.Client) ClientWrapper {
	return ClientWrapper{
		client:      client,
		bulkService: s,
		version:     version,
		clientV8:    clientV8,
	}
}

// IndexExists calls this function to internal client.
func (c ClientWrapper) IndexExists(index string) es.IndicesExistsService {
	return WrapESIndicesExistsService(c.client.IndexExists(index))
}

// CreateIndex calls this function to internal client.
func (c ClientWrapper) CreateIndex(index string) es.IndicesCreateService {
	return WrapESIndicesCreateService(c.client.CreateIndex(index))
}

// DeleteIndex calls this function to internal client.
func (c ClientWrapper) DeleteIndex(index string) es.IndicesDeleteService {
	return WrapESIndicesDeleteService(c.client.DeleteIndex(index))
}

// CreateTemplate calls this function to internal client.
func (c ClientWrapper) CreateTemplate(ttype string) es.TemplateCreateService {
	if c.version.UsesV8API() {
		return TemplateCreatorWrapperV8{
			indicesV8:    c.clientV8.Indices,
			templateName: ttype,
		}
	}
	return WrapESTemplateCreateService(c.client.IndexPutTemplate(ttype))
}

// CreateComponentTemplate creates or updates a composable component template,
// always targeting the _component_template API (data streams require it). It uses
// the v8 client on the v8 API and the olivere client otherwise.
func (c ClientWrapper) CreateComponentTemplate(ctx context.Context, name, template string) error {
	if c.version.UsesV8API() {
		put := c.clientV8.Cluster.PutComponentTemplate
		resp, err := put(name, strings.NewReader(template), put.WithContext(ctx))
		return v8TemplateError(name, resp, err)
	}
	_, err := c.client.IndexPutComponentTemplate(name).BodyString(template).Do(ctx)
	return err
}

// CreateComposableIndexTemplate creates or updates a composable index template
// (the _index_template API). Unlike CreateTemplate (legacy _template on ES 7.x /
// OpenSearch), this is always composable, as data streams require. See
// CreateComponentTemplate for the v7/v8 dispatch.
func (c ClientWrapper) CreateComposableIndexTemplate(ctx context.Context, name, template string) error {
	if c.version.UsesV8API() {
		put := c.clientV8.Indices.PutIndexTemplate
		resp, err := put(name, strings.NewReader(template), put.WithContext(ctx))
		return v8TemplateError(name, resp, err)
	}
	_, err := c.client.IndexPutIndexTemplate(name).BodyString(template).Do(ctx)
	return err
}

// v8TemplateError turns a v8 esapi template-PUT result into an error, accepting
// any 2xx status (a creation may return 201 rather than 200).
func v8TemplateError(name string, resp *esv8api.Response, err error) error {
	if err != nil {
		return fmt.Errorf("error creating template %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("error creating template %s: %s", name, resp)
	}
	return nil
}

// Index calls this function to internal client.
func (c ClientWrapper) Index() es.IndexService {
	r := elastic.NewBulkIndexRequest()
	return WrapESIndexService(r, c.bulkService, c.version)
}

// Search calls this function to internal client.
func (c ClientWrapper) Search(indices ...string) es.SearchService {
	searchService := c.client.Search(indices...)
	if !c.version.SupportsTypedIndices() {
		searchService = searchService.RestTotalHitsAsInt(true)
	}
	return WrapESSearchService(searchService)
}

// MultiSearch calls this function to internal client.
func (c ClientWrapper) MultiSearch() es.MultiSearchService {
	multiSearchService := c.client.MultiSearch()
	return WrapESMultiSearchService(multiSearchService)
}

// Close closes ESClient and flushes all data to the storage.
func (c ClientWrapper) Close() error {
	c.client.Stop()
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

// Do calls this function to internal service.
func (c IndicesCreateServiceWrapper) Do(ctx context.Context) (*elastic.IndicesCreateResult, error) {
	return c.indicesCreateService.Do(ctx)
}

// TemplateCreateServiceWrapper is a wrapper around elastic.IndicesPutTemplateService.
type TemplateCreateServiceWrapper struct {
	mappingCreateService *elastic.IndicesPutTemplateService
}

// IndicesDeleteServiceWrapper is a wrapper around elastic.IndicesDeleteService
type IndicesDeleteServiceWrapper struct {
	indicesDeleteService *elastic.IndicesDeleteService
}

// WrapESIndicesDeleteService creates an ESIndicesDeleteService out of *elastic.IndicesDeleteService.
func WrapESIndicesDeleteService(indicesDeleteService *elastic.IndicesDeleteService) IndicesDeleteServiceWrapper {
	return IndicesDeleteServiceWrapper{indicesDeleteService: indicesDeleteService}
}

// Do calls this function to internal service.
func (e IndicesDeleteServiceWrapper) Do(ctx context.Context) (*elastic.IndicesDeleteResponse, error) {
	return e.indicesDeleteService.Do(ctx)
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

// TemplateCreatorWrapperV8 implements es.TemplateCreateService.
type TemplateCreatorWrapperV8 struct {
	indicesV8       *esv8api.Indices
	templateName    string
	templateMapping string
}

// Body adds mapping to the future request.
func (c TemplateCreatorWrapperV8) Body(mapping string) es.TemplateCreateService {
	cc := c // clone
	cc.templateMapping = mapping
	return cc
}

// Do executes Put Template command.
func (c TemplateCreatorWrapperV8) Do(context.Context) (*elastic.IndicesPutTemplateResponse, error) {
	resp, err := c.indicesV8.PutIndexTemplate(c.templateName, strings.NewReader(c.templateMapping))
	if err != nil {
		return nil, fmt.Errorf("error creating index template %s: %w", c.templateName, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error creating index template %s: %s", c.templateName, resp)
	}
	return nil, nil // no response expected by span writer
}

// ---

// IndexServiceWrapper is a wrapper around elastic.ESIndexService.
// See wrapper_nolint.go for more functions.
type IndexServiceWrapper struct {
	bulkIndexReq *elastic.BulkIndexRequest
	bulkService  *elastic.BulkProcessor
	version      es.BackendVersion
}

// WrapESIndexService creates an ESIndexService out of *elastic.ESIndexService.
func WrapESIndexService(indexService *elastic.BulkIndexRequest, bulkService *elastic.BulkProcessor, version es.BackendVersion) IndexServiceWrapper {
	return IndexServiceWrapper{bulkIndexReq: indexService, bulkService: bulkService, version: version}
}

// Index calls this function to internal service.
func (i IndexServiceWrapper) Index(index string) es.IndexService {
	return WrapESIndexService(i.bulkIndexReq.Index(index), i.bulkService, i.version)
}

// Type calls this function to internal service.
func (i IndexServiceWrapper) Type(typ string) es.IndexService {
	if !i.version.SupportsTypedIndices() {
		return WrapESIndexService(i.bulkIndexReq, i.bulkService, i.version)
	}
	return WrapESIndexService(i.bulkIndexReq.Type(typ), i.bulkService, i.version)
}

// OpType sets the bulk operation type on the request.
func (i IndexServiceWrapper) OpType(opType es.WriteOpType) es.IndexService {
	return WrapESIndexService(i.bulkIndexReq.OpType(string(opType)), i.bulkService, i.version)
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
