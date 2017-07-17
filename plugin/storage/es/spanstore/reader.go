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
	"encoding/json"
	"fmt"
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	jConverter "github.com/uber/jaeger/model/converter/json"
	jModel "github.com/uber/jaeger/model/json"
	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/storage/spanstore"
	storageMetrics "github.com/uber/jaeger/storage/spanstore/metrics"
)

const (
	indexPrefix        = "jaeger-"
	traceIDAggregation = "traceIDs"

	traceIDField       = "traceID"
	durationField      = "duration"
	startTimeField     = "startTime"
	serviceNameField   = "process.serviceName"
	operationNameField = "operationName"
	tagsField          = "tags"
	processTagsField   = "process.tags"
	logFieldsField     = "logs.fields"
	tagKeyField        = "key"
	tagValueField      = "value"

	defaultDocCount  = 10000 // the default elasticsearch allowed limit
	defaultNumTraces = 100
)

var (
	// ErrServiceNameNotSet occurs when attempting to query with an empty service name
	ErrServiceNameNotSet = errors.New("Service Name must be set")

	// ErrStartTimeMinGreaterThanMax occurs when start time min is above start time max
	ErrStartTimeMinGreaterThanMax = errors.New("Start Time Minimum is above Maximum")

	// ErrDurationMinGreaterThanMax occurs when duration min is above duration max
	ErrDurationMinGreaterThanMax = errors.New("Duration Minimum is above Maximum")

	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("Malformed request object")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("Start and End Time must be set")

	// ErrUnableToFindTraceIDAggregation occurs when an aggregation query for TraceIDs fail.
	ErrUnableToFindTraceIDAggregation = errors.New("Could not find aggregation of traceIDs")

	defaultMaxDuration = model.DurationAsMicroseconds(time.Hour * 24)

	tagFieldList = []string{tagsField, processTagsField, logFieldsField}
)

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	ctx    context.Context
	client es.Client
	logger *zap.Logger
	// The age of the oldest service/operation we will look for. Because indices in ElasticSearch are by day,
	// this will be rounded down to UTC 00:00 of that day.
	maxLookback             time.Duration
	serviceOperationStorage *ServiceOperationStorage
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(client es.Client, logger *zap.Logger, maxSpanAge time.Duration, metricsFactory metrics.Factory) spanstore.Reader {
	return storageMetrics.NewReadMetricsDecorator(newSpanReader(client, logger, maxSpanAge), metricsFactory)
}

func newSpanReader(client es.Client, logger *zap.Logger, maxSpanAge time.Duration) *SpanReader {
	ctx := context.Background()
	return &SpanReader{
		ctx:                     ctx,
		client:                  client,
		logger:                  logger,
		maxLookback:             maxSpanAge,
		serviceOperationStorage: NewServiceOperationStorage(ctx, client, metrics.NullFactory, logger, 0), // the decorator takes care of metrics
	}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	return s.readTrace(traceID.String(), &spanstore.TraceQueryParameters{})
}

func (s *SpanReader) readTrace(traceID string, traceQuery *spanstore.TraceQueryParameters) (*model.Trace, error) {
	query := elastic.NewTermQuery(traceIDField, traceID)

	traceQuery.StartTimeMax = traceQuery.StartTimeMax.Add(time.Hour)
	traceQuery.StartTimeMin = traceQuery.StartTimeMin.Add(-time.Hour)
	indices := findIndices(traceQuery.StartTimeMin, traceQuery.StartTimeMax)
	esSpansRaw, err := s.executeQuery(query, indices...)
	if err != nil {
		return nil, errors.Wrap(err, "Query execution failed")
	}
	if len(esSpansRaw) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}

	spans, err := s.collectSpans(esSpansRaw)
	if err != nil {
		return nil, errors.Wrap(err, "Span collection failed")
	}
	return &model.Trace{Spans: spans}, nil
}

func (s *SpanReader) collectSpans(esSpansRaw []*elastic.SearchHit) ([]*model.Span, error) {
	spans := make([]*model.Span, len(esSpansRaw))

	for i, esSpanRaw := range esSpansRaw {
		jsonSpan, err := s.unmarshalJSONSpan(esSpanRaw)
		if err != nil {
			return nil, errors.Wrap(err, "Marshalling JSON to span object failed")
		}
		span, err := jConverter.SpanToDomain(jsonSpan)
		if err != nil {
			return nil, errors.Wrap(err, "Converting JSONSpan to domain Span failed")
		}
		spans[i] = span
	}
	return spans, nil
}

func (s *SpanReader) executeQuery(query elastic.Query, indices ...string) ([]*elastic.SearchHit, error) {
	searchService, err := s.client.Search(indices...).
		Type(spanType).
		Query(query).
		IgnoreUnavailable(true).
		Do(s.ctx)
	if err != nil {
		return nil, err
	}
	return searchService.Hits.Hits, nil
}

func (s *SpanReader) unmarshalJSONSpan(esSpanRaw *elastic.SearchHit) (*jModel.Span, error) {
	esSpanInByteArray := esSpanRaw.Source

	var jsonSpan jModel.Span
	if err := json.Unmarshal(*esSpanInByteArray, &jsonSpan); err != nil {
		return nil, err
	}
	return &jsonSpan, nil
}

// Returns the array of indices that we need to query, based on query params
func findIndices(startTime time.Time, endTime time.Time) []string {
	var indices []string
	firstIndex := IndexWithDate(startTime)
	currentIndex := IndexWithDate(endTime)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		endTime = endTime.Add(-24 * time.Hour)
		currentIndex = IndexWithDate(endTime)
	}
	return append(indices, firstIndex)
}

// IndexWithDate returns the index name formatted to date.
func IndexWithDate(date time.Time) string {
	return indexPrefix + date.UTC().Format("2006-01-02")
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices() ([]string, error) {
	currentTime := time.Now()
	jaegerIndices := findIndices(currentTime.Add(-s.maxLookback), currentTime)
	return s.serviceOperationStorage.getServices(jaegerIndices)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(service string) ([]string, error) {
	currentTime := time.Now()
	jaegerIndices := findIndices(currentTime.Add(-s.maxLookback), currentTime)
	return s.serviceOperationStorage.getOperations(jaegerIndices, service)
}

func bucketToStringArray(buckets []*elastic.AggregationBucketKeyItem) ([]string, error) {
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
	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}
	uniqueTraceIDs, err := s.findTraceIDs(traceQuery)
	if err != nil {
		return nil, err
	}
	var retMe []*model.Trace
	for _, traceID := range uniqueTraceIDs {
		if len(retMe) >= traceQuery.NumTraces {
			break
		}
		trace, err := s.readTrace(traceID, traceQuery)
		if err != nil {
			s.logger.Error("Failure to read trace", zap.String("trace_id", string(traceID)), zap.Error(err))
			continue
		}
		retMe = append(retMe, trace)
	}
	return retMe, nil
}

func validateQuery(p *spanstore.TraceQueryParameters) error {
	if p == nil {
		return ErrMalformedRequestObject
	}
	if p.ServiceName == "" && len(p.Tags) > 0 {
		return ErrServiceNameNotSet
	}
	if p.StartTimeMin.IsZero() || p.StartTimeMax.IsZero() {
		return ErrStartAndEndTimeNotSet
	}
	if p.StartTimeMax.Before(p.StartTimeMin) {
		return ErrStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return ErrDurationMinGreaterThanMax
	}
	return nil
}

func (s *SpanReader) findTraceIDs(traceQuery *spanstore.TraceQueryParameters) ([]string, error) {
	//  Below is the JSON body to our HTTP GET request to ElasticSearch. This function creates this.
	// {
	//      "size": 0,
	//      "query": {
	//        "bool": {
	//          "must": [
	//            { "match": { "operationName":   "op1"      }},
	//            { "match": { "process.serviceName": "service1" }},
	//            { "range":  { "startTime": { "gte": 0, "lte": 90000000000000000 }}},
	//            { "range":  { "duration": { "gte": 0, "lte": 90000000000000000 }}},
	//            { "should": [
	//                   { "nested" : {
	//                      "path" : "tags",
	//                      "query" : {
	//                          "bool" : {
	//                              "must" : [
	//                              { "match" : {"tags.key" : "tag3"} },
	//                              { "match" : {"tags.value" : "xyz"} }
	//                              ]
	//                          }}}},
	//                   { "nested" : {
	//                          "path" : "process.tags",
	//                          "query" : {
	//                              "bool" : {
	//                                  "must" : [
	//                                  { "match" : {"tags.key" : "tag3"} },
	//                                  { "match" : {"tags.value" : "xyz"} }
	//                                  ]
	//                              }}}},
	//                   { "nested" : {
	//                          "path" : "logs.fields",
	//                          "query" : {
	//                              "bool" : {
	//                                  "must" : [
	//                                  { "match" : {"tags.key" : "tag3"} },
	//                                  { "match" : {"tags.value" : "xyz"} }
	//                                  ]
	//                              }}}}
	//                ]
	//              }
	//          ]
	//        }
	//      },
	//      "aggs": { "traceIDs" : { "terms" : {"size": 100,"field": "traceID" }}}
	//  }
	aggregation := s.buildTraceIDAggregation(traceQuery.NumTraces)
	boolQuery := s.buildFindTraceIDsQuery(traceQuery)

	jaegerIndices := findIndices(traceQuery.StartTimeMin, traceQuery.StartTimeMax)

	searchService := s.client.Search(jaegerIndices...).
		Type(spanType).
		Size(0). // set to 0 because we don't want actual documents.
		Aggregation(traceIDAggregation, aggregation).
		IgnoreUnavailable(true).
		Query(boolQuery)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
	}

	bucket, found := searchResult.Aggregations.Terms(traceIDAggregation)
	if !found {
		return nil, ErrUnableToFindTraceIDAggregation
	}

	traceIDBuckets := bucket.Buckets
	return bucketToStringArray(traceIDBuckets)
}

func (s *SpanReader) buildTraceIDAggregation(numOfTraces int) elastic.Aggregation {
	return elastic.NewTermsAggregation().
		Size(numOfTraces).
		Field(traceIDField)
}

func (s *SpanReader) buildFindTraceIDsQuery(traceQuery *spanstore.TraceQueryParameters) elastic.Query {
	boolQuery := elastic.NewBoolQuery()

	//add duration query
	if traceQuery.DurationMax != 0 || traceQuery.DurationMin != 0 {
		durationQuery := s.buildDurationQuery(traceQuery.DurationMin, traceQuery.DurationMax)
		boolQuery.Must(durationQuery)
	}

	//add startTime query
	startTimeQuery := s.buildStartTimeQuery(traceQuery.StartTimeMin, traceQuery.StartTimeMax)
	boolQuery.Must(startTimeQuery)

	//add process.serviceName query
	if traceQuery.ServiceName != "" {
		serviceNameQuery := s.buildServiceNameQuery(traceQuery.ServiceName)
		boolQuery.Must(serviceNameQuery)
	}

	//add operationName query
	if traceQuery.OperationName != "" {
		operationNameQuery := s.buildOperationNameQuery(traceQuery.OperationName)
		boolQuery.Must(operationNameQuery)
	}

	for k, v := range traceQuery.Tags {
		tagQuery := s.buildTagQuery(k, v)
		boolQuery.Must(tagQuery)
	}
	return boolQuery
}

func (s *SpanReader) buildDurationQuery(durationMin time.Duration, durationMax time.Duration) elastic.Query {
	minDurationMicros := model.DurationAsMicroseconds(durationMin)
	maxDurationMicros := defaultMaxDuration
	if durationMax != 0 {
		maxDurationMicros = model.DurationAsMicroseconds(durationMax)
	}
	return elastic.NewRangeQuery(durationField).Gte(minDurationMicros).Lte(maxDurationMicros)
}

func (s *SpanReader) buildStartTimeQuery(startTimeMin time.Time, startTimeMax time.Time) elastic.Query {
	minStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMin)
	maxStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMax)
	return elastic.NewRangeQuery(startTimeField).Gte(minStartTimeMicros).Lte(maxStartTimeMicros)
}

func (s *SpanReader) buildServiceNameQuery(serviceName string) elastic.Query {
	return elastic.NewMatchQuery(serviceNameField, serviceName)
}

func (s *SpanReader) buildOperationNameQuery(operationName string) elastic.Query {
	return elastic.NewMatchQuery(operationNameField, operationName)
}

func (s *SpanReader) buildTagQuery(k string, v string) elastic.Query {
	queries := make([]elastic.Query, len(tagFieldList))
	for i := range queries {
		queries[i] = s.buildNestedQuery(tagFieldList[i], k, v)
	}
	return elastic.NewBoolQuery().Should(queries...)
}

func (s *SpanReader) buildNestedQuery(field string, k string, v string) elastic.Query {
	keyField := fmt.Sprintf("%s.%s", field, tagKeyField)
	valueField := fmt.Sprintf("%s.%s", field, tagValueField)
	keyQuery := elastic.NewMatchQuery(keyField, k)
	valueQuery := elastic.NewMatchQuery(valueField, v)
	tagBoolQuery := elastic.NewBoolQuery().Must(keyQuery, valueQuery)
	return elastic.NewNestedQuery(field, tagBoolQuery)
}
