// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const (
	serviceName = "serviceName"

	operationsAggregation = "distinct_operations"
	servicesAggregation   = "distinct_services"
)

// errNoSearcher is returned by the read methods (getServices/getOperations) when
// the storage was constructed without a searcher — the write-only case described
// on NewServiceOperationStorage. It turns that misconfiguration into a clear
// error rather than a nil-pointer panic.
var errNoSearcher = errors.New("service/operation reads require a searcher, but this storage was constructed for write-only use")

// ServiceOperationStorage stores service to operation pairs.
type ServiceOperationStorage struct {
	searcher     esclient.Searcher
	logger       *zap.Logger
	serviceCache cache.Cache
}

// NewServiceOperationStorage returns a new ServiceOperationStorage. searcher is
// used only by the read methods (getServices/getOperations); a write-only instance
// (the SpanWriter) may pass a nil searcher. The write side builds documents via
// toUpsertItem and commits the cache via commitToCache — it does not write directly.
func NewServiceOperationStorage(
	searcher esclient.Searcher,
	logger *zap.Logger,
	cacheTTL time.Duration,
) *ServiceOperationStorage {
	return &ServiceOperationStorage{
		searcher: searcher,
		logger:   logger,
		serviceCache: cache.NewLRUWithOptions(
			100000,
			&cache.Options{
				TTL: cacheTTL,
			},
		),
	}
}

// toUpsertItem returns the service:operation document to upsert for a span and its
// cache key, or ok=false if the pair is already cached (nothing to write). The
// caller writes the item through its bulk sink and calls commitToCache only after
// the write is durable (RFC 0007 §4.3): marking the cache before durability would
// let a failed-then-retried batch skip the service document and leave a gap. The
// document's _id is a deterministic hash, so a re-sent service doc upserts.
func (s *ServiceOperationStorage) toUpsertItem(indexName string, jsonSpan *dbmodel.Span) (esclient.BulkItem, string, bool) {
	service := dbmodel.Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		OperationName: jsonSpan.OperationName,
	}
	cacheKey := hashCode(service)
	if s.serviceCache.Get(cacheKey) != nil {
		return esclient.BulkItem{}, "", false
	}
	return esclient.BulkItem{Index: indexName, ID: cacheKey, Body: service}, cacheKey, true
}

// commitToCache records that a service:operation document is durably written, so it
// is not re-sent until the cache entry expires. Called only after a successful flush.
func (s *ServiceOperationStorage) commitToCache(cacheKey string) {
	s.serviceCache.Put(cacheKey, cacheKey)
}

// serviceOperationBatch accumulates the new service:operation documents for one write
// batch. It dedups within the batch — the global cache is committed only after a
// durable write (§4.3), so without this many spans sharing a service:operation pair
// would each append the same doc and bloat the request — and, once the batch is
// durable, commits those docs to the cache so later batches skip them.
type serviceOperationBatch struct {
	store *ServiceOperationStorage
	keys  map[string]struct{}
}

func newServiceOperationBatch(store *ServiceOperationStorage) serviceOperationBatch {
	return serviceOperationBatch{store: store, keys: make(map[string]struct{})}
}

// toUpsertItem returns the service:operation document to upsert for a span, or
// ok=false if the pair is already cached globally or was already added earlier in
// this batch.
func (b serviceOperationBatch) toUpsertItem(indexName string, jsonSpan *dbmodel.Span) (esclient.BulkItem, bool) {
	item, cacheKey, ok := b.store.toUpsertItem(indexName, jsonSpan)
	if !ok {
		return esclient.BulkItem{}, false
	}
	if _, seen := b.keys[cacheKey]; seen {
		return esclient.BulkItem{}, false
	}
	b.keys[cacheKey] = struct{}{}
	return item, true
}

// commitToCache marks every service:operation added in this batch as durably
// written. Call it only after the batch write succeeds.
func (b serviceOperationBatch) commitToCache() {
	for key := range b.keys {
		b.store.commitToCache(key)
	}
}

func (s *ServiceOperationStorage) getServices(ctx context.Context, indices []string, maxDocCount int) ([]string, error) {
	if s.searcher == nil {
		return nil, errNoSearcher
	}
	resp, err := s.searcher.Search(ctx, indices, esclient.SearchRequest{
		Size: 0, // aggregation only; we don't want the documents themselves
		Aggregations: map[string]query.Aggregation{
			// Size bounds distinct buckets — ES deprecated size omission for
			// aggregating all. https://github.com/elastic/elasticsearch/issues/18838
			servicesAggregation: query.NewTermsAggregation(serviceName).Size(maxDocCount),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("search services failed: %w", err)
	}
	return aggregationKeys(resp, servicesAggregation)
}

func (s *ServiceOperationStorage) getOperations(ctx context.Context, indices []string, service string, maxDocCount int) ([]string, error) {
	if s.searcher == nil {
		return nil, errNoSearcher
	}
	resp, err := s.searcher.Search(ctx, indices, esclient.SearchRequest{
		Size:  0,
		Query: query.NewTermQuery(serviceName, service),
		Aggregations: map[string]query.Aggregation{
			operationsAggregation: query.NewTermsAggregation(operationNameField).Size(maxDocCount),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("search operations failed: %w", err)
	}
	return aggregationKeys(resp, operationsAggregation)
}

// aggregationKeys extracts the bucket keys of a named terms aggregation. A
// response with no aggregations yields an empty slice; a response missing the
// requested aggregation is an error.
func aggregationKeys(resp *esclient.SearchResponse, name string) ([]string, error) {
	if resp == nil {
		return nil, errors.New("nil search response")
	}
	if resp.Aggregations == nil {
		return []string{}, nil
	}
	agg, ok := resp.Aggregations.Terms(name)
	if !ok {
		return nil, errors.New("could not find aggregation of " + name)
	}
	keys := make([]string, 0, len(agg.Buckets))
	for _, bucket := range agg.Buckets {
		keys = append(keys, bucket.Key)
	}
	return keys, nil
}

func hashCode(s dbmodel.Service) string {
	h := fnv.New64a()
	h.Write([]byte(s.ServiceName))
	h.Write([]byte(s.OperationName))
	return strconv.FormatUint(h.Sum64(), 16)
}
