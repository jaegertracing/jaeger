// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package spanstore

import (
	"context"
	"fmt"
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	jModel "github.com/uber/jaeger/model/json"
	"github.com/uber/jaeger/pkg/cache"
	"github.com/uber/jaeger/pkg/es"
	storageMetrics "github.com/uber/jaeger/storage/spanstore/metrics"
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
		ctx:     ctx,
		client:  client,
		metrics: storageMetrics.NewWriteMetrics(metricsFactory, "ServiceOperation"),
		logger:  logger,
		serviceCache: cache.NewLRUWithOptions(
			100000,
			&cache.Options{
				TTL: cacheTTL,
			},
		),
	}
}

// Write saves a service to operation pair.
func (s *ServiceOperationStorage) Write(indexName string, jsonSpan *jModel.Span) error {
	// Insert serviceName:operationName document
	service := Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		OperationName: jsonSpan.OperationName,
	}
	serviceID := fmt.Sprintf("%s|%s", service.ServiceName, service.OperationName)
	cacheKey := fmt.Sprintf("%s:%s", indexName, serviceID)
	if !keyInCache(cacheKey, s.serviceCache) {
		start := time.Now()
		_, err := s.client.Index().Index(indexName).Type(serviceType).Id(serviceID).BodyJson(service).Do(s.ctx)
		s.metrics.Emit(err, time.Since(start))
		if err != nil {
			return s.logError(jsonSpan, err, "Failed to insert service:operation", s.logger)
		}
		writeCache(cacheKey, s.serviceCache)
	}
	return nil
}

func (s *ServiceOperationStorage) getServices(indices []string) ([]string, error) {
	serviceAggregation := getServicesAggregation()

	searchService := s.client.Search(indices...).
		Type(serviceType).
		Size(0). // set to 0 because we don't want actual documents.
		Aggregation(servicesAggregation, serviceAggregation)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
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
		Aggregation(operationsAggregation, serviceFilter)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
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

func (s *ServiceOperationStorage) logError(span *jModel.Span, err error, msg string, logger *zap.Logger) error {
	logger.Debug("trace info:", zap.String("trace_id", string(span.TraceID)), zap.String("span_id", string(span.SpanID)))
	logger.Error(msg, zap.Error(err))
	return errors.Wrap(err, msg)
}
