// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
)

type IndexAPI interface {
	GetJaegerIndices(ctx context.Context, prefix string) ([]Index, error)
	IndexExists(ctx context.Context, index string) (bool, error)
	AliasExists(ctx context.Context, alias string) (bool, error)
	DeleteIndices(ctx context.Context, indices []Index) error
	CreateIndex(ctx context.Context, index string) error
	CreateAlias(ctx context.Context, aliases []Alias) error
	DeleteAlias(ctx context.Context, aliases []Alias) error
	CreateTemplate(ctx context.Context, name string, mappingType MappingType) error
	Rollover(ctx context.Context, rolloverTarget string, conditions map[string]any) error
}

type IndexManagementLifecycleAPI interface {
	Exists(ctx context.Context, name string) (bool, error)
}

// Searcher runs searches against Elasticsearch/OpenSearch.
type Searcher interface {
	Search(ctx context.Context, indices []string, req SearchRequest) (*SearchResponse, error)
}

// BulkWriter enqueues documents for writing via the bulk API. It is the narrow
// surface callers depend on; the concrete indexer's lifecycle (Close) is owned
// by whoever constructs it (the factory).
type BulkWriter interface {
	Add(item BulkItem)
}
