// Copyright (c) 2019 The Jaeger Authors.
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

package spanstore

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/olivere/elastic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

const (
	serviceName = "serviceName"

	operationsAggregation = "distinct_operations"
	servicesAggregation   = "distinct_services"
)

// ServiceOperationStorage stores service to operation pairs.
type ServiceOperationStorage struct {
	client       es.Client
	logger       *zap.Logger
	serviceCache cache.Cache
}

// NewServiceOperationStorage returns a new ServiceOperationStorage.
func NewServiceOperationStorage(
	client es.Client,
	logger *zap.Logger,
	cacheTTL time.Duration,
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
		s.client.Index().Index(indexName).Type(serviceType).Id(cacheKey).BodyJson(service).Add()
		writeCache(cacheKey, s.serviceCache)
	}
}

func (s *ServiceOperationStorage) getServices(context context.Context, indices []string, maxDocCount int) ([]string, error) {
	serviceAggregation := getServicesAggregation(maxDocCount)

	searchService := s.client.Search(indices...).
		Size(0). // set to 0 because we don't want actual documents.
		IgnoreUnavailable(true).
		Aggregation(servicesAggregation, serviceAggregation)

	searchResult, err := searchService.Do(context)
	if err != nil {
		return nil, fmt.Errorf("search services failed: %w", err)
	}
	if searchResult.Aggregations == nil {
		return []string{}, nil
	}
	bucket, found := searchResult.Aggregations.Terms(servicesAggregation)
	if !found {
		return nil, errors.New("could not find aggregation of " + servicesAggregation)
	}
	serviceNamesBucket := bucket.Buckets
	return bucketToStringArray(serviceNamesBucket)
}

func getServicesAggregation(maxDocCount int) elastic.Query {
	return elastic.NewTermsAggregation().
		Field(serviceName).
		Size(maxDocCount) // ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

func (s *ServiceOperationStorage) getOperations(context context.Context, indices []string, service string, maxDocCount int) ([]string, error) {
	serviceQuery := elastic.NewTermQuery(serviceName, service)
	serviceFilter := getOperationsAggregation(maxDocCount)

	searchService := s.client.Search(indices...).
		Size(0).
		Query(serviceQuery).
		IgnoreUnavailable(true).
		Aggregation(operationsAggregation, serviceFilter)

	searchResult, err := searchService.Do(context)
	if err != nil {
		return nil, fmt.Errorf("search operations failed: %w", err)
	}
	if searchResult.Aggregations == nil {
		return []string{}, nil
	}
	bucket, found := searchResult.Aggregations.Terms(operationsAggregation)
	if !found {
		return nil, errors.New("could not find aggregation of " + operationsAggregation)
	}
	operationNamesBucket := bucket.Buckets
	return bucketToStringArray(operationNamesBucket)
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
	return fmt.Sprintf("%x", h.Sum64())
}
