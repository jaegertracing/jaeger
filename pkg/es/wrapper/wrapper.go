// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package eswrapper

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	esV8 "github.com/elastic/go-elasticsearch/v8"
	esV8api "github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/olivere/elastic"

	"github.com/jaegertracing/jaeger/pkg/es"
)

// This file avoids lint because the Id and Json are required to be capitalized, but must match an outside library.

// ClientWrapper is a wrapper around elastic.Client
type ClientWrapper struct {
	client      *elastic.Client
	bulkService *elastic.BulkProcessor
	esVersion   uint
	clientV8    *esV8.Client
}

// GetVersion returns the ElasticSearch Version
func (c ClientWrapper) GetVersion() uint {
	return c.esVersion
}

// WrapESClient creates a ESClient out of *elastic.Client.
func WrapESClient(client *elastic.Client, s *elastic.BulkProcessor, esVersion uint, clientV8 *esV8.Client) ClientWrapper {
	return ClientWrapper{
		client:      client,
		bulkService: s,
		esVersion:   esVersion,
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

// GetIndices call this function to internal client
func (c ClientWrapper) GetIndices() es.IndicesGetService {
	indicesGetService := elastic.NewIndicesGetService(c.client)
	return WrapIndicesGetService(indicesGetService)
}

// CreateAlias calls the AliasService in the internal client with AddAction induced in it
func (c ClientWrapper) CreateAlias(alias string) es.AliasAddAction {
	aliasAddAction := elastic.NewAliasAddAction(alias)
	return WrapAliasAddAction(aliasAddAction, c.client)
}

// DeleteAlias calls the AliasService in the internal client with RemoveAction induced in it
func (c ClientWrapper) DeleteAlias(alias string) es.AliasRemoveAction {
	aliasRemoveAction := elastic.NewAliasRemoveAction(alias)
	return WrapAliasRemoveAction(aliasRemoveAction, c.client)
}

// CreateIlmPolicy calls the internal XPackIlmPutLifecycle service
func (c ClientWrapper) CreateIlmPolicy() es.XPackIlmPutLifecycle {
	xPack := elastic.NewXPackIlmPutLifecycleService(c.client)
	return WrapXPackIlmPutLifecycle(xPack)
}

// CreateIsmPolicy creates the Ism Policy which is similar to ILM Policy (but not same) for OpenSearch
func (c ClientWrapper) CreateIsmPolicy(ctx context.Context, id, policy string) (*elastic.Response, error) {
	return c.client.PerformRequest(ctx, elastic.PerformRequestOptions{
		Path:   "/_plugins/_ism/policies/" + id,
		Method: http.MethodPut,
		Body:   policy,
	})
}

// IlmPolicyExists returns true if policy exists and returns false if not
func (c ClientWrapper) IlmPolicyExists(ctx context.Context, id string) (bool, error) {
	ilmGetService := elastic.NewXPackIlmGetLifecycleService(c.client)
	_, err := ilmGetService.Policy(id).Do(ctx)
	if err != nil {
		if elastic.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsmPolicyExists returns true if policy exists and returns false if not
func (c ClientWrapper) IsmPolicyExists(ctx context.Context, id string) (bool, error) {
	_, err := c.client.PerformRequest(ctx, elastic.PerformRequestOptions{
		Path:   "/_plugins/_ism/policies/" + id,
		Method: http.MethodGet,
	})
	if err != nil {
		if elastic.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CreateTemplate calls this function to internal client.
func (c ClientWrapper) CreateTemplate(ttype string) es.TemplateCreateService {
	if c.esVersion >= 8 {
		return TemplateCreatorWrapperV8{
			indicesV8:    c.clientV8.Indices,
			templateName: ttype,
		}
	}
	return WrapESTemplateCreateService(c.client.IndexPutTemplate(ttype))
}

// Index calls this function to internal client.
func (c ClientWrapper) Index() es.IndexService {
	r := elastic.NewBulkIndexRequest()
	return WrapESIndexService(r, c.bulkService, c.esVersion)
}

// Search calls this function to internal client.
func (c ClientWrapper) Search(indices ...string) es.SearchService {
	searchService := c.client.Search(indices...)
	if c.esVersion >= 7 {
		searchService = searchService.RestTotalHitsAsInt(true)
	}
	return WrapESSearchService(searchService)
}

// MultiSearch calls this function to internal client.
func (c ClientWrapper) MultiSearch() es.MultiSearchService {
	multiSearchService := c.client.MultiSearch()
	if c.esVersion >= 7 {
		multiSearchService = multiSearchService.RestTotalHitsAsInt(true)
	}
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
	indicesV8       *esV8api.Indices
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
	esVersion    uint
}

// WrapESIndexService creates an ESIndexService out of *elastic.ESIndexService.
func WrapESIndexService(indexService *elastic.BulkIndexRequest, bulkService *elastic.BulkProcessor, esVersion uint) IndexServiceWrapper {
	return IndexServiceWrapper{bulkIndexReq: indexService, bulkService: bulkService, esVersion: esVersion}
}

// Index calls this function to internal service.
func (i IndexServiceWrapper) Index(index string) es.IndexService {
	return WrapESIndexService(i.bulkIndexReq.Index(index), i.bulkService, i.esVersion)
}

// Type calls this function to internal service.
func (i IndexServiceWrapper) Type(typ string) es.IndexService {
	if i.esVersion >= 7 {
		return WrapESIndexService(i.bulkIndexReq, i.bulkService, i.esVersion)
	}
	return WrapESIndexService(i.bulkIndexReq.Type(typ), i.bulkService, i.esVersion)
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

// AliasAddActionWrapper is a wrapper around elastic.AliasAddAction
type AliasAddActionWrapper struct {
	aliasAddAction *elastic.AliasAddAction
	client         *elastic.Client
}

// WrapAliasAddAction creates an AliasAddActionWrapper out of *elastic.AliasAddAction.
func WrapAliasAddAction(aliasAddAction *elastic.AliasAddAction, client *elastic.Client) AliasAddActionWrapper {
	return AliasAddActionWrapper{aliasAddAction: aliasAddAction, client: client}
}

// Index calls this function to internal service.
func (a AliasAddActionWrapper) Index(index ...string) es.AliasAddAction {
	return WrapAliasAddAction(a.aliasAddAction.Index(index...), a.client)
}

// IsWriteIndex calls this function to internal service.
func (a AliasAddActionWrapper) IsWriteIndex(flag bool) es.AliasAddAction {
	return WrapAliasAddAction(a.aliasAddAction.IsWriteIndex(flag), a.client)
}

// Do calls this function to internal service.
func (a AliasAddActionWrapper) Do(ctx context.Context) (*elastic.AliasResult, error) {
	return a.client.Alias().Action(a.aliasAddAction).Do(ctx)
}

// AliasRemoveActionWrapper is a wrapper around elastic.AliasRemoveAction
type AliasRemoveActionWrapper struct {
	aliasRemoveAction *elastic.AliasRemoveAction
	client            *elastic.Client
}

// WrapAliasRemoveAction creates an AliasRemoveActionWrapper out of *elastic.AliasRemoveAction.
func WrapAliasRemoveAction(aliasRemoveAction *elastic.AliasRemoveAction, client *elastic.Client) AliasRemoveActionWrapper {
	return AliasRemoveActionWrapper{aliasRemoveAction: aliasRemoveAction, client: client}
}

// Index calls this function to internal service.
func (a AliasRemoveActionWrapper) Index(index ...string) es.AliasRemoveAction {
	return WrapAliasRemoveAction(a.aliasRemoveAction.Index(index...), a.client)
}

// Do calls this function to internal service.
func (a AliasRemoveActionWrapper) Do(ctx context.Context) (*elastic.AliasResult, error) {
	return a.client.Alias().Action(a.aliasRemoveAction).Do(ctx)
}

// XPackIlmPutLifecycleWrapper is a wrapper around elastic.XPackIlmPutLifecycleService
type XPackIlmPutLifecycleWrapper struct {
	xPackPutLifecycleWrapper *elastic.XPackIlmPutLifecycleService
}

// WrapXPackIlmPutLifecycle creates an AliasRemoveActionWrapper out of *elastic.XPackIlmPutLifecycleService.
func WrapXPackIlmPutLifecycle(xPackIlmPutLifecycleWrapper *elastic.XPackIlmPutLifecycleService) XPackIlmPutLifecycleWrapper {
	return XPackIlmPutLifecycleWrapper{xPackPutLifecycleWrapper: xPackIlmPutLifecycleWrapper}
}

// BodyString calls this function to internal service.
func (x XPackIlmPutLifecycleWrapper) BodyString(body string) es.XPackIlmPutLifecycle {
	return WrapXPackIlmPutLifecycle(x.xPackPutLifecycleWrapper.BodyString(body))
}

// Policy calls this function to internal service.
func (x XPackIlmPutLifecycleWrapper) Policy(policy string) es.XPackIlmPutLifecycle {
	return WrapXPackIlmPutLifecycle(x.xPackPutLifecycleWrapper.Policy(policy))
}

// Do calls this function to internal service.
func (x XPackIlmPutLifecycleWrapper) Do(ctx context.Context) (*elastic.XPackIlmPutLifecycleResponse, error) {
	return x.xPackPutLifecycleWrapper.Do(ctx)
}

// IndicesGetServiceWrapper is a wrapper around elastic.IndicesGetService
type IndicesGetServiceWrapper struct {
	indicesGetService *elastic.IndicesGetService
}

// WrapIndicesGetService creates an AliasRemoveActionWrapper out of *elastic.IndicesGetService.
func WrapIndicesGetService(indicesGetService *elastic.IndicesGetService) IndicesGetServiceWrapper {
	return IndicesGetServiceWrapper{indicesGetService: indicesGetService}
}

// Index calls this function to internal service.
func (i IndicesGetServiceWrapper) Index(indices ...string) es.IndicesGetService {
	return WrapIndicesGetService(i.indicesGetService.Index(indices...))
}

// Do calls this function to internal service.
func (i IndicesGetServiceWrapper) Do(ctx context.Context) (map[string]*elastic.IndicesGetResponse, error) {
	return i.indicesGetService.Do(ctx)
}
