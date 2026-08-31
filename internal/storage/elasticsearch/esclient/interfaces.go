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

// Searcher runs searches against Elasticsearch/OpenSearch: single _search
// requests and batched _msearch requests (the paginated trace read uses the
// latter to fetch many traces in one round trip).
type Searcher interface {
	Search(ctx context.Context, indices []string, req SearchRequest) (*SearchResponse, error)
	MultiSearch(ctx context.Context, reqs []MultiSearchRequest) ([]SearchResponse, error)
}

// BatchWriter is the single interface for writing documents via the bulk API: it
// writes a whole batch in one call and returns an error if the write failed. Both
// bulk writers implement it — the async BulkIndexer enqueues the batch
// fire-and-forget (returning nil; an enqueue cannot fail synchronously, and per-item
// failures surface in its callbacks), and the SyncBulkWriter issues one blocking
// _bulk and returns the real per-batch error (RFC 0007). Single-document callers
// pass a one-element batch. The concrete indexer's lifecycle (Close) is owned by
// whoever constructs it (the factory).
type BatchWriter interface {
	WriteBatch(ctx context.Context, items []BulkItem) error
}
