// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"io"

	"github.com/olivere/elastic/v7"
)

// Client is an abstraction for elastic.Client
type Client interface {
	IndexExists(ctx context.Context, index string) IndicesExistsService
	CreateIndex(ctx context.Context, index string) IndicesCreateService
	CreateTemplate(ctx context.Context, id string) TemplateCreateService
	Index() IndexService
	Search(ctx context.Context, indices ...string) SearchService
	MultiSearch(ctx context.Context) MultiSearchService
	DeleteIndex(ctx context.Context, index string) IndicesDeleteService
	io.Closer
	GetVersion() BackendVersion
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

// WriteOpType is the Elasticsearch/OpenSearch bulk operation type.
type WriteOpType string

const (
	// WriteOpIndex is the standard "index" operation (upsert semantics).
	WriteOpIndex WriteOpType = "index"

	// WriteOpCreate is the "create" operation (fail if document exists).
	// Required by data streams, which are append-only.
	WriteOpCreate WriteOpType = "create"
)

// IndexService is an abstraction for elastic BulkService
type IndexService interface {
	Index(index string) IndexService
	Type(typ string) IndexService
	Id(id string) IndexService
	BodyJson(body any) IndexService
	// OpType sets the bulk operation type. Data streams require WriteOpCreate;
	// legacy indices use the default WriteOpIndex.
	OpType(opType WriteOpType) IndexService
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
