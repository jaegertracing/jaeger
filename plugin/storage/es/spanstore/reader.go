package spanstore

import (
	"context"
	"errors"
	"time"

	"github.com/olivere/elastic"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
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
	threeDaysAgo := today.AddDate(0, 0, -3)

	if traceQuery.StartTimeMax.IsZero() || traceQuery.StartTimeMin.IsZero() {
		traceQuery.StartTimeMax = time.Now()
		traceQuery.StartTimeMin = time.Now().AddDate(0, 0, -3)
	}

	var indices []string
	current := traceQuery.StartTimeMax
	for current.After(traceQuery.StartTimeMin) && current.After(threeDaysAgo) {
		index := "jaeger-" + current.Format("2006-01-02")
		exists, _ := s.client.IndexExists(index).Do(s.ctx)
		if exists {
			indices = append(indices, index)
		}
		current = current.AddDate(0, 0, -1)
	}
	return indices
}
// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices() ([]string, error) {
	serviceAggregation := s.getServicesAggregation()

	jaegerIndices := s.findIndices(spanstore.TraceQueryParameters{})

	searchService := s.client.Search(jaegerIndices...).
		Type(serviceType).
		Size(0). // set to 0 because we don't want actual documents.
		Aggregation("distinct_services", serviceAggregation)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, err
	}

	bucket, found := searchResult.Aggregations.Terms("distinct_services")
	if !found {
		err = errors.New("Could not find aggregation of services")
		return nil, err
	}
	serviceNamesBucket := bucket.Buckets
	return s.bucketToStringArray(serviceNamesBucket)
}

func (s *SpanReader) getServicesAggregation() elastic.Query {
	return elastic.NewTermsAggregation().
		Field("serviceName").
		Size(3000) // Must set to some large number. ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(service string) ([]string, error) {
	serviceQuery := elastic.NewTermQuery("serviceName", service)
	serviceFilter := elastic.NewFilterAggregation().Filter(serviceQuery)
	jaegerIndices := s.findIndices(spanstore.TraceQueryParameters{})

	searchService := s.client.Search(jaegerIndices...).
		Type(serviceType).
		Size(0).
		Aggregation("distinct_operations", serviceFilter)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, err
	}
	bucket, found := searchResult.Aggregations.Terms("distinct_operations")
	if !found {
		err = errors.New("Could not find aggregation of operations")
		return nil, err
	}
	operationNamesBucket := bucket.Buckets
	return s.bucketToStringArray(operationNamesBucket)
}

func (s *SpanReader) bucketToStringArray(buckets []*elastic.AggregationBucketKeyItem) ([]string, error) {
	strings := make([]string, len(buckets))
	for i, keyitem := range buckets {
		s, ok := keyitem.Key.(string)
		if !ok {
			return nil, errors.New("Non-string key found in aggregation")
		}
		strings[i] = s
	}
	return strings, nil
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return nil, nil
}