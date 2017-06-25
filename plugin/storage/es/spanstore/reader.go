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
	"time"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	jConverter "github.com/uber/jaeger/model/converter/json"
	jModel "github.com/uber/jaeger/model/json"
	"github.com/uber/jaeger/pkg/es"
	"github.com/uber/jaeger/storage/spanstore"
)

const (
	serviceName           = "serviceName"
	indexPrefix           = "jaeger-"
	operationsAggregation = "distinct_operations"
	servicesAggregation   = "distinct_services"
	traceIDAggregation    = "traceIDs"
	traceIDField          = "traceID"
	durationField         = "duration"
	startTimeField        = "startTime"
	serviceNameField      = "process.serviceName"
	operationNameField    = "operationName"
	tagKeyField           = "tags.key"
	tagValueField         = "tags.value"
	tagsField             = "tags"

	defaultDocCount  = 3000
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

	// ErrDurationAndTagQueryNotSupported occurs when duration and tags are both set
	ErrDurationAndTagQueryNotSupported = errors.New("Cannot query for duration and tags simultaneously")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("Start and End Time must be set")
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
	return s.readTrace(traceID.String(), &spanstore.TraceQueryParameters{})
}

func (s *SpanReader) readTrace(traceID string, traceQuery *spanstore.TraceQueryParameters) (*model.Trace, error) {
	query := elastic.NewTermQuery(traceIDField, traceID)

	indices := s.findIndices(traceQuery)
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
		jsonSpan, err := s.unmarshallJSONSpan(esSpanRaw)
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
		Do(s.ctx)
	if err != nil {
		return nil, err
	}
	return searchService.Hits.Hits, nil
}

func (s *SpanReader) unmarshallJSONSpan(esSpanRaw *elastic.SearchHit) (*jModel.Span, error) {
	esSpanInByteArray := esSpanRaw.Source

	var jsonSpan jModel.Span
	if err := json.Unmarshal(*esSpanInByteArray, &jsonSpan); err != nil {
		return nil, err
	}
	return &jsonSpan, nil
}

// Returns the array of indices that we need to query, based on query params
func (s *SpanReader) findIndices(traceQuery *spanstore.TraceQueryParameters) []string {
	today := time.Now()
	threeDaysAgo := today.AddDate(0, 0, -3) // TODO: make this configurable

	if traceQuery.StartTimeMax.IsZero() || traceQuery.StartTimeMin.IsZero() {
		traceQuery.StartTimeMax = today
		traceQuery.StartTimeMin = threeDaysAgo
	}

	var indices []string
	current := traceQuery.StartTimeMax
	for current.After(traceQuery.StartTimeMin) && current.After(threeDaysAgo) {
		index := indexWithDate(current)
		exists, _ := s.client.IndexExists(index).Do(s.ctx) // Don't care about error, if it's an error, exists will be false anyway
		if exists {
			indices = append(indices, index)
		}
		current = current.AddDate(0, 0, -1)
	}
	return indices
}

func indexWithDate(date time.Time) string {
	return indexPrefix + date.Format("2006-01-02")
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices() ([]string, error) {
	serviceAggregation := s.getServicesAggregation()

	jaegerIndices := s.findIndices(&spanstore.TraceQueryParameters{})

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
		return nil, errors.New("Could not find aggregation of " + servicesAggregation)
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
	serviceFilter := s.getOperationsAggregation()
	jaegerIndices := s.findIndices(&spanstore.TraceQueryParameters{})

	searchService := s.client.Search(jaegerIndices...).
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
	return s.bucketToStringArray(operationNamesBucket)
}

func (s *SpanReader) getOperationsAggregation() elastic.Query {
	return elastic.NewTermsAggregation().
		Field(operationNameField).
		Size(defaultDocCount) // Must set to some large number. ES deprecated size omission for aggregating all. https://github.com/elastic/elasticsearch/issues/18838
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
	for traceID := range uniqueTraceIDs {
		if len(retMe) >= traceQuery.NumTraces {
			break
		}
		trace, err := s.readTrace(string(traceID), traceQuery)
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
	if !p.StartTimeMin.IsZero() && !p.StartTimeMax.IsZero() && p.StartTimeMax.Before(p.StartTimeMin) {
		return ErrStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return ErrDurationMinGreaterThanMax
	}
	if (p.DurationMin != 0 || p.DurationMax != 0) && len(p.Tags) > 0 {
		return ErrDurationAndTagQueryNotSupported
	}
	return nil
}

func (s *SpanReader) findTraceIDs(traceQuery *spanstore.TraceQueryParameters) ([]string, error) {
	//  Below is the JSON body to our HTTP GET request to ElasticSearch. This function creates this.
	//
	// {
	//     "size": 0,
	//     "query": {
	//       "bool": {
	//         "must": [
	//           { "match": { "operationName":   "traceQuery.OperationName"      }},
	//           { "match": { "process.serviceName": "traceQuery.ServiceName" }},
	//           { "range":  { "timestamp": { "gte": traceQuery.StartTimeMin, "lte": traceQuery.StartTimeMax }}},
	//           { "range":  { "duration": { "gte": traceQuery.DurationMin, "lte": traceQuery.DurationMax }}},
	//           { "nested" : {
	//             "path" : "tags",
	//             "query" : {
	//                 "bool" : {
	//                     "must" : [
	//                     { "match" : {"tags.key" : "traceQuery.Tags.key"} },
	//                     { "match" : {"tags.value" : "traceQuery.Tags.value"} }
	//                     ]
	//                 }}}}
	//         ]
	//       }
	//     },
	//     "aggs": { "traceIDs" : { "terms" : {"size": numOfTraces,"field": "traceID" }}}
	// }
	aggregation := s.buildTraceIDAggregation(traceQuery.NumTraces)
	boolQuery := s.buildFindTraceIDsQuery(traceQuery)

	jaegerIndices := s.findIndices(traceQuery)

	searchService := s.client.Search(jaegerIndices...).
		Type(spanType).
		Size(0). // set to 0 because we don't want actual documents.
		Aggregation(traceIDAggregation, aggregation).
		Query(boolQuery)

	searchResult, err := searchService.Do(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Search service failed")
	}

	bucket, found := searchResult.Aggregations.Terms(traceIDAggregation)
	if !found {
		err = errors.New("Could not find aggregation of traceIDs")
		return nil, err
	}

	traceIDBuckets := bucket.Buckets
	return s.bucketToStringArray(traceIDBuckets)
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

	//add tags query (must be nested) TODO: add log tags query
	for k, v := range traceQuery.Tags {
		tagQuery := s.buildTagQuery(k, v)
		boolQuery.Must(tagQuery)
	}
	return boolQuery
}

func (s *SpanReader) buildDurationQuery(durationMin time.Duration, durationMax time.Duration) elastic.Query {
	minDurationMicros := durationMin.Nanoseconds() / int64(time.Microsecond/time.Nanosecond)
	maxDurationMicros := (time.Hour * 24).Nanoseconds() / int64(time.Microsecond/time.Nanosecond)
	if durationMax != 0 {
		maxDurationMicros = durationMax.Nanoseconds() / int64(time.Microsecond/time.Nanosecond)
	}
	return elastic.NewRangeQuery(durationField).Gte(minDurationMicros).Lte(maxDurationMicros)
}

func (s *SpanReader) buildStartTimeQuery(startTimeMin time.Time, startTimeMax time.Time) elastic.Query {
	minStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMin)
	maxStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMin.Add(24 * time.Hour))
	if !startTimeMax.IsZero() {
		maxStartTimeMicros = model.TimeAsEpochMicroseconds(startTimeMax)
	}
	return elastic.NewRangeQuery(startTimeField).Gte(minStartTimeMicros).Lte(maxStartTimeMicros)
}

func (s *SpanReader) buildServiceNameQuery(serviceName string) elastic.Query {
	return elastic.NewMatchQuery(serviceNameField, serviceName)
}

func (s *SpanReader) buildOperationNameQuery(operationName string) elastic.Query {
	return elastic.NewMatchQuery(operationNameField, operationName)
}

func (s *SpanReader) buildTagQuery(k string, v string) elastic.Query {
	keyQuery := elastic.NewMatchQuery(tagKeyField, k)
	valueQuery := elastic.NewMatchQuery(tagValueField, v)
	tagBoolQuery := elastic.NewBoolQuery().Must(keyQuery, valueQuery)
	return elastic.NewNestedQuery(tagsField, tagBoolQuery)
}
