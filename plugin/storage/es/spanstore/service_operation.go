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
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/olivere/elastic.v5"

	jModel "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

const (
	serviceName = "serviceName"

	operationsAggregation = "distinct_operations"
	servicesAggregation   = "distinct_services"
)

// ServiceOperationStorage stores service to operation pairs.
type ServiceOperationStorage struct {
	ctx          context.Context
	client       es.Client
	metrics      *storageMetrics.WriteMetrics
	logger       *zap.Logger
	serviceCache cache.Cache
}

// NewServiceOperationStorage returns a new ServiceOperationStorage.
func NewServiceOperationStorage(
	ctx context.Context,
	client es.Client,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	cacheTTL time.Duration,
) *ServiceOperationStorage {
	return &ServiceOperationStorage{
		ctx:    ctx,
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
func (s *ServiceOperationStorage) Write(indexName string, jsonSpan *jModel.Span) {
	// Insert serviceName:operationName document
	service := Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		OperationName: jsonSpan.OperationName,
	}
	serviceID := fmt.Sprintf("%s|%s", service.ServiceName, service.OperationName)
	cacheKey := fmt.Sprintf("%s:%s", indexName, serviceID)
	if !keyInCache(cacheKey, s.serviceCache) {
		s.client.Index().Index(indexName).Type(serviceType).Id(serviceID).BodyJson(service).Add()
		writeCache(cacheKey, s.serviceCache)
	}
}

func (s *ServiceOperationStorage) getServices(indices []string) ([]string, error) {
	serviceAggregation := getServicesAggregation()

	searchService := s.client.Search(indices...).
		Type(serviceType).
		Size(0). // set to 0 because we don't want actual documents.
		IgnoreUnavailable(true).
		Aggregation(servicesAggregation, serviceAggregation)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
	}
	if searchResult.Aggregations == nil {
		return []string{}, nil
	}
	bucket, found := searchResult.Aggregations.Terms(servicesAggregation)
	if !found {
		return nil, errors.New("Could not find aggregation of " + servicesAggregation)
	}
	serviceNamesBucket := bucket.Buckets
	return bucketToStringArray(serviceNamesBucket)
}

func getServicesAggregation() elastic.Query {
	return elastic.NewTermsAggregation().
		Field(serviceName).
		Size(defaultDocCount) // Must set to some large number. ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

func (s *ServiceOperationStorage) getOperations(indices []string, service string) ([]string, error) {
	serviceQuery := elastic.NewTermQuery(serviceName, service)
	serviceFilter := getOperationsAggregation()

	searchService := s.client.Search(indices...).
		Type(serviceType).
		Size(0).
		Query(serviceQuery).
		IgnoreUnavailable(true).
		Aggregation(operationsAggregation, serviceFilter)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
	}
	if searchResult.Aggregations == nil {
		return []string{}, nil
	}
	bucket, found := searchResult.Aggregations.Terms(operationsAggregation)
	if !found {
		return nil, errors.New("Could not find aggregation of " + operationsAggregation)
	}
	operationNamesBucket := bucket.Buckets
	return bucketToStringArray(operationNamesBucket)
}

func getOperationsAggregation() elastic.Query {
	return elastic.NewTermsAggregation().
		Field(operationNameField).
		Size(defaultDocCount) // Must set to some large number. ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}
