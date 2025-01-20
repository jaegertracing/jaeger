// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"context"
	"io"

	"github.com/olivere/elastic"
)

// Client is an abstraction for elastic.Client
type Client interface {
	IndexExists(index string) IndicesExistsService
	CreateIndex(index string) IndicesCreateService
	GetIndices() IndicesGetService
	CreateTemplate(id string) TemplateCreateService
	CreateAlias(name string) AliasAddAction
	DeleteAlias(name string) AliasRemoveAction
	CreateIlmPolicy() XPackIlmPutLifecycle
	CreateIsmPolicy(ctx context.Context, id string, policy string) (*elastic.Response, error)
	IlmPolicyExists(ctx context.Context, id string) (bool, error)
	IsmPolicyExists(ctx context.Context, id string) (bool, error)
	Index() IndexService
	Search(indices ...string) SearchService
	MultiSearch() MultiSearchService
	DeleteIndex(index string) IndicesDeleteService
	io.Closer
	GetVersion() uint
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

// IndicesDeleteService is an abstraction for elastic.IndicesDeleteService
type IndicesDeleteService interface {
	Do(ctx context.Context) (*elastic.IndicesDeleteResponse, error)
}

// TemplateCreateService is an abstraction for creating a mapping
type TemplateCreateService interface {
	Body(mapping string) TemplateCreateService
	Do(ctx context.Context) (*elastic.IndicesPutTemplateResponse, error)
}

// IndexService is an abstraction for elastic BulkService
type IndexService interface {
	Index(index string) IndexService
	Type(typ string) IndexService
	Id(id string) IndexService
	BodyJson(body any) IndexService
	Add()
}

// SearchService is an abstraction for elastic.SearchService
type SearchService interface {
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

type AliasAddAction interface {
	Index(index ...string) AliasAddAction
	IsWriteIndex(flag bool) AliasAddAction
	Do(ctx context.Context) (*elastic.AliasResult, error)
}

type AliasRemoveAction interface {
	Index(index ...string) AliasRemoveAction
	Do(ctx context.Context) (*elastic.AliasResult, error)
}

type XPackIlmPutLifecycle interface {
	BodyString(body string) XPackIlmPutLifecycle
	Policy(policy string) XPackIlmPutLifecycle
	Do(ctx context.Context) (*elastic.XPackIlmPutLifecycleResponse, error)
}

type IndicesGetService interface {
	Index(indices ...string) IndicesGetService
	Do(ctx context.Context) (map[string]*elastic.IndicesGetResponse, error)
}
