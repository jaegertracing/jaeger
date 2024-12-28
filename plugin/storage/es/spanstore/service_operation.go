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

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/internal/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	serviceName           = "serviceName"
	spanKind              = "spanKind"
	operationsAggregation = "distinct_operations"
	operationsWithoutKind = "distinct_operations_without_kind"
	servicesAggregation   = "distinct_services"
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
		OperationName: jsonSpan.OperationName,
	}
	if kind != model.SpanKindUnspecified {
		serviceWithKind := dbmodel.ServiceWithKind{
			Service: service,
			Kind:    string(kind),
		}
		cacheKey := hashCodeWithKind(serviceWithKind)
		if !keyInCache(cacheKey, s.serviceCache) {
			s.client().Index().Index(indexName).Type(serviceType).Id(cacheKey).BodyJson(serviceWithKind).Add()
			writeCache(cacheKey, s.serviceCache)
		}
	} else {
		cacheKey := hashCode(service)
		if !keyInCache(cacheKey, s.serviceCache) {
			s.client().Index().Index(indexName).Type(serviceType).Id(cacheKey).BodyJson(service).Add()
			writeCache(cacheKey, s.serviceCache)
		}
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

func (s *ServiceOperationStorage) getOperations(ctx context.Context, indices []string, service, kind string, maxDocCount int) ([]spanstore.Operation, error) {
	var searchService es.SearchService
	if kind != "" {
		serviceFilter := getOperationsAggregation(maxDocCount)
		serviceNameQuery := elastic.NewTermQuery(serviceName, service)
		serviceKindQuery := elastic.NewTermQuery(spanKind, kind)
		serviceQuery := elastic.NewBoolQuery().Must(serviceNameQuery, serviceKindQuery)
		searchService = s.client().Search(indices...).
			Size(0).
			Query(serviceQuery).
			IgnoreUnavailable(true).
			Aggregation(operationsAggregation, serviceFilter)
		searchResult, err := searchService.Do(ctx)
		if err != nil {
			return nil, fmt.Errorf("search operations failed: %w", es.DetailedError(err))
		}
		if searchResult.Aggregations == nil {
			return []spanstore.Operation{}, nil
		}
		bucket, found := searchResult.Aggregations.Terms(operationsAggregation)
		if !found {
			return nil, errors.New("could not find aggregation of " + operationsAggregation)
		}
		operationNamesBucket := bucket.Buckets
		return bucketOfOperationNamesToOperationsArray(operationNamesBucket, kind)
	}
	serviceFilter := elastic.NewFiltersAggregation().
		FilterWithName(string(model.SpanKindClient), elastic.NewTermQuery(spanKind, string(model.SpanKindClient))).
		FilterWithName(string(model.SpanKindServer), elastic.NewTermQuery(spanKind, string(model.SpanKindServer))).
		FilterWithName(string(model.SpanKindProducer), elastic.NewTermQuery(spanKind, string(model.SpanKindProducer))).
		FilterWithName(string(model.SpanKindConsumer), elastic.NewTermQuery(spanKind, string(model.SpanKindConsumer))).
		FilterWithName(string(model.SpanKindInternal), elastic.NewTermQuery(spanKind, string(model.SpanKindInternal))).
		FilterWithName(operationsWithoutKind, elastic.NewBoolQuery().MustNot(elastic.NewExistsQuery(spanKind))).
		SubAggregation(operationNameField, elastic.NewTermsAggregation().Field(operationNameField).Size(maxDocCount))
	serviceQuery := elastic.NewTermQuery(serviceName, service)
	searchService = s.client().Search(indices...).
		Size(0).
		Query(serviceQuery).
		IgnoreUnavailable(true).
		Aggregation(operationsAggregation, serviceFilter)
	searchResult, err := searchService.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search operations failed: %w", es.DetailedError(err))
	}
	if searchResult.Aggregations == nil {
		return []spanstore.Operation{}, nil
	}
	bucket, found := searchResult.Aggregations.Filters(operationsAggregation)
	if !found {
		return nil, errors.New("could not find aggregation of " + operationsAggregation)
	}
	return bucketOfOperationsToOperationsArray(bucket)
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

func bucketOfOperationsToOperationsArray(searchResult *elastic.AggregationBucketFilters) ([]spanstore.Operation, error) {
	var result []spanstore.Operation
	for name, bucket := range searchResult.NamedBuckets {
		if kind, err := model.SpanKindFromString(name); err == nil {
			if v, ok := bucket.Aggregations[operationNameField]; ok {
				result, err = addOperationsFromRawData(v, string(kind), result)
				if err != nil {
					return nil, err
				}
			}
		} else {
			if name == operationsWithoutKind {
				if v, ok := bucket.Aggregations[operationNameField]; ok {
					result, err = addOperationsFromRawData(v, "", result)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return result, nil
}

func addOperationsFromRawData(raw *json.RawMessage, kind string, result []spanstore.Operation) ([]spanstore.Operation, error) {
	var items *elastic.AggregationBucketKeyItems
	err := json.Unmarshal(*raw, &items)
	if err != nil {
		return nil, err
	}
	for _, item := range items.Buckets {
		str, ok := item.Key.(string)
		if !ok {
			return nil, errors.New("non-string key found in aggregation")
		}
		result = append(result, spanstore.Operation{
			Name:     str,
			SpanKind: kind,
		})
	}
	return result, nil
}

func hashCode(s dbmodel.Service) string {
	h := fnv.New64a()
	h.Write([]byte(s.ServiceName))
	h.Write([]byte(s.OperationName))
	return strconv.FormatUint(h.Sum64(), 16)
}

func hashCodeWithKind(s dbmodel.ServiceWithKind) string {
	h := fnv.New64a()
	h.Write([]byte(s.ServiceName))
	h.Write([]byte(s.Kind))
	h.Write([]byte(s.OperationName))
	return strconv.FormatUint(h.Sum64(), 16)
}
