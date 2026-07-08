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
	bulkWriter   esclient.BulkWriter
	logger       *zap.Logger
	serviceCache cache.Cache
}

// NewServiceOperationStorage returns a new ServiceOperationStorage. searcher is
// used only by the read methods (getServices/getOperations) and bulkWriter only
// by Write; a read-only instance may pass a nil bulkWriter and a write-only
// instance a nil searcher.
func NewServiceOperationStorage(
	searcher esclient.Searcher,
	bulkWriter esclient.BulkWriter,
	logger *zap.Logger,
	cacheTTL time.Duration,
) *ServiceOperationStorage {
	return &ServiceOperationStorage{
		searcher:   searcher,
		bulkWriter: bulkWriter,
		logger:     logger,
		serviceCache: cache.NewLRUWithOptions(
			100000,
			&cache.Options{
				TTL: cacheTTL,
			},
		),
	}
}

// Write saves a service to operation pair. It is a no-op on a read-only
// instance (nil bulk writer, see NewServiceOperationStorage), since the same
// type backs both the read and write paths.
func (s *ServiceOperationStorage) Write(indexName string, jsonSpan *dbmodel.Span) {
	if s.bulkWriter == nil {
		s.logger.Error("cannot write service:operation pair: storage was constructed for read-only use")
		return
	}
	// Insert serviceName:operationName document
	service := dbmodel.Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		OperationName: jsonSpan.OperationName,
	}

	cacheKey := hashCode(service)
	if !keyInCache(cacheKey, s.serviceCache) {
		s.bulkWriter.Add(esclient.BulkItem{
			Index: indexName,
			ID:    cacheKey,
			Body:  service,
		})
		writeCache(cacheKey, s.serviceCache)
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
