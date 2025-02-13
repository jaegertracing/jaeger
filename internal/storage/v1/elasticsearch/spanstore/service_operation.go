// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/olivere/elastic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	"github.com/jaegertracing/jaeger/pkg/es"
)

const (
	serviceName         = "serviceName"
	spanKindField       = "spanKind"
	operationsAggName   = "distinct_operations"
	servicesAggregation = "distinct_services"
)

// ServiceOperationStorage stores service to operation pairs.
type ServiceOperationStorage struct {
	client       func() es.Client
	logger       *zap.Logger
	serviceCache cache.Cache
}

// NewServiceOperationStorage returns a new ServiceOperationStorage.
func NewServiceOperationStorage(
	client func() es.Client,
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
func (s *ServiceOperationStorage) Write(indexName string, jsonSpan *dbmodel.Span, kind model.SpanKind) {
	// Insert serviceName:operationName document
	service := dbmodel.Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		Kind:          string(kind),
		OperationName: jsonSpan.OperationName,
	}
	cacheKey := hashCode(service)
	if !keyInCache(cacheKey, s.serviceCache) {
		s.client().Index().Index(indexName).Type(serviceType).Id(cacheKey).BodyJson(service).Add()
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
	return bucketToStringArray(serviceNamesBucket)
}

func getServicesAggregation(maxDocCount int) elastic.Query {
	return elastic.NewTermsAggregation().
		Field(serviceName).
		Size(maxDocCount) // ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

func (s *ServiceOperationStorage) getOperationsWithKind(ctx context.Context, indices []string, service, kind string, maxDocCount int) ([]spanstore.Operation, error) {
	var searchService es.SearchService
	if kind != "" {
		searchService = s.client().Search(indices...).
			Size(0). // set to 0 because we don't want actual documents.
			Query(elastic.NewBoolQuery().Must(
				elastic.NewTermQuery(serviceName, service),
				elastic.NewTermQuery(spanKindField, kind))).
			IgnoreUnavailable(true).
			Aggregation(operationsAggName, getOperationsAggregation(maxDocCount))
		searchResult, err := searchService.Do(ctx)
		if err != nil {
			return nil, fmt.Errorf("search operations failed: %w", es.DetailedError(err))
		}
		if searchResult.Aggregations == nil {
			return []spanstore.Operation{}, nil
		}
		bucket, found := searchResult.Aggregations.Terms(operationsAggName)
		if !found {
			return nil, errors.New("could not find aggregation of " + operationsAggName)
		}
		operationNamesBucket := bucket.Buckets
		return bucketOfOperationNamesToOperationsArray(operationNamesBucket, kind)
	}
	serviceQuery := elastic.NewTermQuery(serviceName, service)
	searchService = s.client().Search(indices...).
		Query(serviceQuery).
		IgnoreUnavailable(true).
		FetchSourceContext(elastic.NewFetchSourceContext(true).Include(spanKindField, operationNameField)).
		Size(maxDocCount)
	searchResult, err := searchService.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search operations failed: %w", es.DetailedError(err))
	}
	if searchResult.Hits == nil {
		return []spanstore.Operation{}, nil
	}
	return bucketOfOperationsToOperationsArray(searchResult.Hits)
}

func getOperationsAggregation(maxDocCount int) elastic.Query {
	return elastic.NewTermsAggregation().
		Field(operationNameField).
		Size(maxDocCount) // ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

func bucketOfOperationNamesToOperationsArray(buckets []*elastic.AggregationBucketKeyItem, kind string) ([]spanstore.Operation, error) {
	result := make([]spanstore.Operation, len(buckets))
	for i, keyItem := range buckets {
		str, ok := keyItem.Key.(string)
		if !ok {
			return nil, errors.New("non-string key found in aggregation")
		}
		result[i] = spanstore.Operation{
			Name:     str,
			SpanKind: kind,
		}
	}
	return result, nil
}

func bucketOfOperationsToOperationsArray(searchResult *elastic.SearchHits) ([]spanstore.Operation, error) {
	result := make([]spanstore.Operation, len(searchResult.Hits))
	for i, hit := range searchResult.Hits {
		data := hit.Source
		op, err := rawMessageToOperation(data)
		if err != nil {
			return nil, err
		}
		result[i] = op
	}
	return result, nil
}

func rawMessageToOperation(data *json.RawMessage) (spanstore.Operation, error) {
	var service dbmodel.Service
	if err := json.Unmarshal(*data, &service); err != nil {
		return spanstore.Operation{}, err
	}
	return spanstore.Operation{Name: service.OperationName, SpanKind: service.Kind}, nil
}

func hashCode(s dbmodel.Service) string {
	h := fnv.New64a()
	h.Write([]byte(s.ServiceName))
	h.Write([]byte(s.Kind))
	h.Write([]byte(s.OperationName))
	return strconv.FormatUint(h.Sum64(), 16)
}
