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
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/storage/spanstore"
)

const (
	serviceName           = "serviceName"
	indexPrefix           = "jaeger-"
	operationsAggregation = "distinct_operations"
	servicesAggregation   = "distinct_services"
	defaultDocCount       = 3000
)

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	ctx    context.Context
	client es.Client
	logger *zap.Logger
}

// NewSpanReader returns a new SpanReader.
func NewSpanReader(client es.Client, logger *zap.Logger) *SpanReader {
	ctx := context.Background()
	return &SpanReader{
		ctx:    ctx,
		client: client,
		logger: logger,
	}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	// TODO
	return nil, nil
}

// Returns the array of indices that we need to query, based on query params
func (s *SpanReader) findIndices(traceQuery spanstore.TraceQueryParameters) []string {
	today := time.Now()
	threeDaysAgo := today.AddDate(0, 0, -3) // TODO: make this configurable

	if traceQuery.StartTimeMax.IsZero() || traceQuery.StartTimeMin.IsZero() {
		traceQuery.StartTimeMax = today
		traceQuery.StartTimeMin = threeDaysAgo
	}

	var indices []string
	current := traceQuery.StartTimeMax
	for current.After(traceQuery.StartTimeMin) && current.After(threeDaysAgo) {
		index := s.indexWithDate(current)
		exists, _ := s.client.IndexExists(index).Do(s.ctx) // Don't care about error, if it's an error, exists will be false anyway
		if exists {
			indices = append(indices, index)
		}
		current = current.AddDate(0, 0, -1)
	}
	return indices
}

func (s *SpanReader) indexWithDate(date time.Time) string {
	return indexPrefix + date.Format("2006-01-02")
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices() ([]string, error) {
	serviceAggregation := s.getServicesAggregation()

	jaegerIndices := s.findIndices(spanstore.TraceQueryParameters{})

	searchService := s.client.Search(jaegerIndices...).
		Type(serviceType).
		Size(0). // set to 0 because we don't want actual documents.
		Aggregation(servicesAggregation, serviceAggregation)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
	}

	bucket, found := searchResult.Aggregations.Terms(servicesAggregation)
	if !found {
		return nil, errors.New("Could not find aggregation of services")
	}
	serviceNamesBucket := bucket.Buckets
	return s.bucketToStringArray(serviceNamesBucket)
}

func (s *SpanReader) getServicesAggregation() elastic.Query {
	return elastic.NewTermsAggregation().
		Field(serviceName).
		Size(defaultDocCount) // Must set to some large number. ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(service string) ([]string, error) {
	serviceQuery := elastic.NewTermQuery(serviceName, service)
	serviceFilter := elastic.NewFilterAggregation().Filter(serviceQuery)
	jaegerIndices := s.findIndices(spanstore.TraceQueryParameters{})

	searchService := s.client.Search(jaegerIndices...).
		Type(serviceType).
		Size(0).
		Aggregation(operationsAggregation, serviceFilter)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
	}
	bucket, found := searchResult.Aggregations.Terms(operationsAggregation)
	if !found {
		return nil, errors.New("Could not find aggregation of operations")
	}
	operationNamesBucket := bucket.Buckets
	return s.bucketToStringArray(operationNamesBucket)
}

func (s *SpanReader) bucketToStringArray(buckets []*elastic.AggregationBucketKeyItem) ([]string, error) {
	strings := make([]string, len(buckets))
	for i, keyitem := range buckets {
		str, ok := keyitem.Key.(string)
		if !ok {
			return nil, errors.New("Non-string key found in aggregation")
		}
		strings[i] = str
	}
	return strings, nil
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	// TODO
	return nil, nil
}
