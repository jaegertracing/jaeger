// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

const (
	serviceName = "serviceName"

	operationsAggregation = "distinct_operations"
	servicesAggregation   = "distinct_services"
)

// ServiceOperationStorage stores service to operation pairs.
type ServiceOperationStorage struct {
	client        func() es.Client
	logger        *zap.Logger
	serviceCache  cache.Cache
	useDataStream bool
}

// NewServiceOperationStorage returns a new ServiceOperationStorage.
func NewServiceOperationStorage(
	client func() es.Client,
	logger *zap.Logger,
	cacheTTL time.Duration,
	useDataStream bool,
) *ServiceOperationStorage {
	return &ServiceOperationStorage{
		client: client,
		logger: logger,
		serviceCache: cache.NewLRUWithOptions(
			100000,
			&cache.Options{
				TTL: cacheTTL,
			},
		),
		useDataStream: useDataStream,
	}
}

// Write saves a service to operation pair.
func (s *ServiceOperationStorage) Write(indexName string, jsonSpan *dbmodel.Span) {
	// Insert serviceName:operationName document
	service := dbmodel.Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		OperationName: jsonSpan.OperationName,
	}

	cacheKey := hashCode(service)
	if !keyInCache(cacheKey, s.serviceCache) {
		il := s.client().Index().Index(indexName).Type(serviceType).BodyJson(service)
		opType := ""
		if s.useDataStream || s.client().GetVersion() >= 8 {
			opType = "create"
			if !s.useDataStream {
				il.Id(cacheKey)
			}
		} else {
			il.Id(cacheKey)
		}
		il.Add(opType)
		writeCache(cacheKey, s.serviceCache)
	}
}

func (s *ServiceOperationStorage) getServices(ctx context.Context, indices []string, maxDocCount int) ([]string, error) {
	serviceAggregation := getServicesAggregation(maxDocCount)

	searchService := s.client().Search(indices...).
		Size(0). // set to 0 because we don't want actual documents.
		IgnoreUnavailable(true).
		Aggregation(servicesAggregation, serviceAggregation)

	searchResult, err := searchService.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search services failed: %w", es.DetailedError(err))
	}
	if searchResult.Aggregations == nil {
		return []string{}, nil
	}
	bucket, found := searchResult.Aggregations.Terms(servicesAggregation)
	if !found {
		return nil, errors.New("could not find aggregation of " + servicesAggregation)
	}
	serviceNamesBucket := bucket.Buckets
	return bucketToStringArray[string](serviceNamesBucket)
}

func getServicesAggregation(maxDocCount int) elastic.Query {
	return elastic.NewTermsAggregation().
		Field(serviceName).
		Size(maxDocCount) // ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

func (s *ServiceOperationStorage) getOperations(ctx context.Context, indices []string, service string, maxDocCount int) ([]string, error) {
	serviceQuery := elastic.NewTermQuery(serviceName, service)
	serviceFilter := getOperationsAggregation(maxDocCount)

	searchService := s.client().Search(indices...).
		Size(0).
		Query(serviceQuery).
		IgnoreUnavailable(true).
		Aggregation(operationsAggregation, serviceFilter)

	searchResult, err := searchService.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search operations failed: %w", es.DetailedError(err))
	}
	if searchResult.Aggregations == nil {
		return []string{}, nil
	}
	bucket, found := searchResult.Aggregations.Terms(operationsAggregation)
	if !found {
		return nil, errors.New("could not find aggregation of " + operationsAggregation)
	}
	operationNamesBucket := bucket.Buckets
	return bucketToStringArray[string](operationNamesBucket)
}

func getOperationsAggregation(maxDocCount int) elastic.Query {
	return elastic.NewTermsAggregation().
		Field(operationNameField).
		Size(maxDocCount) // ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

func hashCode(s dbmodel.Service) string {
	h := fnv.New64a()
	h.Write([]byte(s.ServiceName))
	h.Write([]byte(s.OperationName))
	return strconv.FormatUint(h.Sum64(), 16)
}
