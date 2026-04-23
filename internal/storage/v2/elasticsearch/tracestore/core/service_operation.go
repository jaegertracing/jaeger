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

	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const (
	serviceName = "serviceName"

	operationsAggregation = "distinct_operations"
	servicesAggregation   = "distinct_services"

	spanKindField       = "spanKind"
	spanKindAggregation = "distinct_span_kinds"
	spanKindTagKey      = "span.kind"
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
func (s *ServiceOperationStorage) Write(indexName string, jsonSpan *dbmodel.Span) {
	// Insert serviceName:spanKind:operationName document
	service := dbmodel.Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		SpanKind:      getSpanKindFromSpan(jsonSpan),
		OperationName: jsonSpan.OperationName,
	}

	cacheKey := hashCode(service)
	if !keyInCache(cacheKey, s.serviceCache) {
		s.client().Index().Index(indexName).Type(serviceType).Id(cacheKey).BodyJson(service).Add()
		writeCache(cacheKey, s.serviceCache)
	}
}

// getSpanKindFromSpan extracts the span kind string from the span's tags.
// The span kind is stored as a "span.kind" tag in OpenTracing/Jaeger spans.
// Returns an empty string if the tag is not present.
func getSpanKindFromSpan(span *dbmodel.Span) string {
	for _, tag := range span.Tags {
		if tag.Key == spanKindTagKey {
			if v, ok := tag.Value.(string); ok {
				return v
			}
		}
	}
	// Fall back to the flat tag map if populated.
	if v, ok := span.Tag[spanKindTagKey]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
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

func (s *ServiceOperationStorage) getOperations(
	ctx context.Context,
	indices []string,
	service string,
	spanKind string,
	maxDocCount int,
) ([]dbmodel.Operation, error) {
	serviceQuery := elastic.NewTermQuery(serviceName, service)

	var query elastic.Query = serviceQuery
	if spanKind != "" {
		query = elastic.NewBoolQuery().Must(
			serviceQuery,
			elastic.NewTermQuery(spanKindField, spanKind),
		)
	}

	searchService := s.client().Search(indices...).
		Size(0).
		Query(query).
		IgnoreUnavailable(true).
		Aggregation(operationsAggregation, getOperationsAggregation(maxDocCount))

	searchResult, err := searchService.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search operations failed: %w", es.DetailedError(err))
	}
	if searchResult.Aggregations == nil {
		return []dbmodel.Operation{}, nil
	}
	bucket, found := searchResult.Aggregations.Terms(operationsAggregation)
	if !found {
		return nil, errors.New("could not find aggregation of " + operationsAggregation)
	}
	return operationsBucketToOperations(bucket.Buckets)
}

// getOperationsAggregation returns a terms aggregation on operationName with a nested
// sub-aggregation on spanKind, so that each unique (operationName, spanKind) pair is returned.
func getOperationsAggregation(maxDocCount int) elastic.Aggregation {
	spanKindAgg := elastic.NewTermsAggregation().
		Field(spanKindField).
		Size(10) // there are at most 7 span kinds defined in the OpenTelemetry spec
	return elastic.NewTermsAggregation().
		Field(operationNameField).
		Size(maxDocCount). // ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
		SubAggregation(spanKindAggregation, spanKindAgg)
}

// operationsBucketToOperations converts Elasticsearch aggregation buckets into Operation slices.
// Each operationName bucket may contain a nested spanKind sub-aggregation; if present, one
// Operation is emitted per (operationName, spanKind) pair. If the sub-aggregation is absent or
// empty (e.g. legacy data without span kind), a single Operation with an empty SpanKind is emitted
// to preserve backward compatibility.
func operationsBucketToOperations(buckets []*elastic.AggregationBucketKeyItem) ([]dbmodel.Operation, error) {
	operations := make([]dbmodel.Operation, 0, len(buckets))
	for _, opBucket := range buckets {
		opName, ok := opBucket.Key.(string)
		if !ok {
			return nil, errors.New("could not convert operation name bucket key to string")
		}
		kindBuckets, found := opBucket.Terms(spanKindAggregation)
		if !found || len(kindBuckets.Buckets) == 0 {
			// Legacy document or data without span kind — emit with empty SpanKind.
			operations = append(operations, dbmodel.Operation{Name: opName})
			continue
		}
		for _, kb := range kindBuckets.Buckets {
			kind, ok := kb.Key.(string)
			if !ok {
				return nil, errors.New("could not convert span kind bucket key to string")
			}
			operations = append(operations, dbmodel.Operation{
				Name:     opName,
				SpanKind: kind,
			})
		}
	}
	return operations, nil
}

func hashCode(s dbmodel.Service) string {
	h := fnv.New64a()
	h.Write([]byte(s.ServiceName))
	h.Write([]byte("|"))
	h.Write([]byte(s.SpanKind))
	h.Write([]byte("|"))
	h.Write([]byte(s.OperationName))
	return strconv.FormatUint(h.Sum64(), 16)
}
