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
	"encoding/json"
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go"
	ottag "github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/olivere/elastic.v5"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	spanIndex          = "jaeger-span-"
	serviceIndex       = "jaeger-service-"
	traceIDAggregation = "traceIDs"

	traceIDField           = "traceID"
	durationField          = "duration"
	startTimeField         = "startTime"
	serviceNameField       = "process.serviceName"
	operationNameField     = "operationName"
	objectTagsField        = "tag"
	objectProcessTagsField = "process.tag"
	nestedTagsField        = "tags"
	nestedProcessTagsField = "process.tags"
	nestedLogFieldsField   = "logs.fields"
	tagKeyField            = "key"
	tagValueField          = "value"

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

	errNoTraces = errors.New("No trace with that ID found")

	defaultMaxDuration = model.DurationAsMicroseconds(time.Hour * 24)

	objectTagFieldList = []string{objectTagsField, objectProcessTagsField}

	nestedTagFieldList = []string{nestedTagsField, nestedProcessTagsField, nestedLogFieldsField}
)

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	ctx    context.Context
	client es.Client
	logger *zap.Logger
	// The age of the oldest service/operation we will look for. Because indices in ElasticSearch are by day,
	// this will be rounded down to UTC 00:00 of that day.
	maxSpanAge              time.Duration
	serviceOperationStorage *ServiceOperationStorage
	spanIndexPrefix         string
	serviceIndexPrefix      string
	spanConverter           dbmodel.ToDomain
	dateFormat              string
}

// SpanReaderParams holds constructor params for NewSpanReader
type SpanReaderParams struct {
	Client                  es.Client
	Logger                  *zap.Logger
	MaxSpanAge              time.Duration
	MetricsFactory          metrics.Factory
	serviceOperationStorage *ServiceOperationStorage
	IndexPrefix             string
	TagDotReplacement       string
	DateFormat              string
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(p SpanReaderParams) spanstore.Reader {
	return newSpanReader(p)
}

func newSpanReader(p SpanReaderParams) *SpanReader {
	ctx := context.Background()
	if p.IndexPrefix != "" {
		p.IndexPrefix += ":"
	}
	return &SpanReader{
		ctx:                     ctx,
		client:                  p.Client,
		logger:                  p.Logger,
		maxSpanAge:              p.MaxSpanAge,
		serviceOperationStorage: NewServiceOperationStorage(ctx, p.Client, p.Logger, 0), // the decorator takes care of metrics
		spanIndexPrefix:         p.IndexPrefix + spanIndex,
		serviceIndexPrefix:      p.IndexPrefix + serviceIndex,
		spanConverter:           dbmodel.NewToDomain(p.TagDotReplacement),
		dateFormat:              p.DateFormat,
	}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetTrace")
	defer span.Finish()
	currentTime := time.Now()
	traces, err := s.multiRead(ctx, []string{traceID.String()}, currentTime.Add(-s.maxSpanAge), currentTime)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, errNoTraces
	}
	return traces[0], nil
}

func (s *SpanReader) collectSpans(esSpansRaw []*elastic.SearchHit) ([]*model.Span, error) {
	spans := make([]*model.Span, len(esSpansRaw))

	for i, esSpanRaw := range esSpansRaw {
		jsonSpan, err := s.unmarshalJSONSpan(esSpanRaw)
		if err != nil {
			return nil, errors.Wrap(err, "Marshalling JSON to span object failed")
		}
		span, err := s.spanConverter.SpanToDomain(jsonSpan)
		if err != nil {
			return nil, errors.Wrap(err, "Converting JSONSpan to domain Span failed")
		}
		spans[i] = span
	}
	return spans, nil
}

func (s *SpanReader) unmarshalJSONSpan(esSpanRaw *elastic.SearchHit) (*dbmodel.Span, error) {
	esSpanInByteArray := esSpanRaw.Source

	var jsonSpan dbmodel.Span
	if err := json.Unmarshal(*esSpanInByteArray, &jsonSpan); err != nil {
		return nil, err
	}
	return &jsonSpan, nil
}

// Returns the array of indices that we need to query, based on query params
func (s *SpanReader) indicesForTimeRange(indexName string, startTime time.Time, endTime time.Time) []string {
	var indices []string
	firstIndex := indexWithDate(indexName, startTime, s.dateFormat)
	currentIndex := indexWithDate(indexName, endTime, s.dateFormat)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		endTime = endTime.Add(-24 * time.Hour)
		currentIndex = indexWithDate(indexName, endTime, s.dateFormat)
	}
	return append(indices, firstIndex)
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetServices")
	defer span.Finish()
	currentTime := time.Now()
	jaegerIndices := s.indicesForTimeRange(s.serviceIndexPrefix, currentTime.Add(-s.maxSpanAge), currentTime)
	return s.serviceOperationStorage.getServices(jaegerIndices)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(ctx context.Context, service string) ([]string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetOperations")
	defer span.Finish()
	currentTime := time.Now()
	jaegerIndices := s.indicesForTimeRange(s.serviceIndexPrefix, currentTime.Add(-s.maxSpanAge), currentTime)
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
func (s *SpanReader) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "FindTraces")
	defer span.Finish()

	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}
	uniqueTraceIDs, err := s.findTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	return s.multiRead(ctx, uniqueTraceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
}

func (s *SpanReader) multiRead(ctx context.Context, traceIDs []string, startTime, endTime time.Time) ([]*model.Trace, error) {

	childSpan, _ := opentracing.StartSpanFromContext(ctx, "multiRead")
	childSpan.LogFields(otlog.Object("trace_ids", traceIDs))
	defer childSpan.Finish()

	if len(traceIDs) == 0 {
		return []*model.Trace{}, nil
	}
	searchRequests := make([]*elastic.SearchRequest, len(traceIDs))

	var traces []*model.Trace
	// Add an hour in both directions so that traces that straddle two indexes are retrieved.
	// i.e starts in one and ends in another.
	indices := s.indicesForTimeRange(s.spanIndexPrefix, startTime.Add(-time.Hour), endTime.Add(time.Hour))

	nextTime := model.TimeAsEpochMicroseconds(startTime.Add(-time.Hour))

	searchAfterTime := make(map[string]uint64)
	totalDocumentsFetched := make(map[string]int)
	tracesMap := make(map[string]*model.Trace)
	for {
		if len(traceIDs) == 0 {
			break
		}

		for i, traceID := range traceIDs {
			query := elastic.NewTermQuery("traceID", traceID)
			if val, ok := searchAfterTime[traceID]; ok {
				nextTime = val
			}
			searchRequests[i] = elastic.NewSearchRequest().IgnoreUnavailable(true).Type(spanType).Source(elastic.NewSearchSource().Query(query).Size(defaultDocCount).Sort("startTime", true).SearchAfter(nextTime))
		}
		// set traceIDs to empty
		traceIDs = nil
		results, err := s.client.MultiSearch().Add(searchRequests...).Index(indices...).Do(s.ctx)

		if err != nil {
			logErrorToSpan(childSpan, err)
			return nil, err
		}

		if results.Responses == nil || len(results.Responses) == 0 {
			break
		}

		for _, result := range results.Responses {
			if result.Hits == nil || len(result.Hits.Hits) == 0 {
				continue
			}
			spans, err := s.collectSpans(result.Hits.Hits)
			if err != nil {
				logErrorToSpan(childSpan, err)
				return nil, err
			}
			lastSpan := spans[len(spans)-1]
			lastSpanTraceID := lastSpan.TraceID.String()

			if traceSpan, ok := tracesMap[lastSpanTraceID]; ok {
				traceSpan.Spans = append(traceSpan.Spans, spans...)
			} else {
				tracesMap[lastSpanTraceID] = &model.Trace{Spans: spans}
			}

			totalDocumentsFetched[lastSpanTraceID] = totalDocumentsFetched[lastSpanTraceID] + len(result.Hits.Hits)
			if totalDocumentsFetched[lastSpanTraceID] < int(result.TotalHits()) {
				traceIDs = append(traceIDs, lastSpanTraceID)
				searchAfterTime[lastSpanTraceID] = model.TimeAsEpochMicroseconds(lastSpan.StartTime)
			}
		}
	}

	for _, trace := range tracesMap {
		traces = append(traces, trace)
	}
	return traces, nil
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

func (s *SpanReader) findTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]string, error) {
	childSpan, _ := opentracing.StartSpanFromContext(ctx, "findTraceIDs")
	defer childSpan.Finish()
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
	//                              }}}},
	//                   { "bool":{
	//                           "must": {
	//                               "match":{ "tags.bat":{ "query":"spook" }}
	//                           }}},
	//                   { "bool":{
	//                           "must": {
	//                               "match":{ "tag.bat":{ "query":"spook" }}
	//                           }}}
	//                ]
	//              }
	//          ]
	//        }
	//      },
	//      "aggs": { "traceIDs" : { "terms" : {"size": 100,"field": "traceID" }}}
	//  }
	aggregation := s.buildTraceIDAggregation(traceQuery.NumTraces)
	boolQuery := s.buildFindTraceIDsQuery(traceQuery)

	jaegerIndices := s.indicesForTimeRange(s.spanIndexPrefix, traceQuery.StartTimeMin, traceQuery.StartTimeMax)

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
	if searchResult.Aggregations == nil {
		return []string{}, nil
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
		Field(traceIDField).
		Order(startTimeField, false).
		SubAggregation(startTimeField, s.buildTraceIDSubAggregation())
}

func (s *SpanReader) buildTraceIDSubAggregation() elastic.Aggregation {
	return elastic.NewMaxAggregation().
		Field(startTimeField)
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
	objectTagListLen := len(objectTagFieldList)
	queries := make([]elastic.Query, len(nestedTagFieldList)+objectTagListLen)
	kd := s.spanConverter.ReplaceDot(k)
	for i := range objectTagFieldList {
		queries[i] = s.buildObjectQuery(objectTagFieldList[i], kd, v)
	}
	for i := range nestedTagFieldList {
		queries[i+objectTagListLen] = s.buildNestedQuery(nestedTagFieldList[i], k, v)
	}

	// but configuration can change over time
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

func (s *SpanReader) buildObjectQuery(field string, k string, v string) elastic.Query {
	keyField := fmt.Sprintf("%s.%s", field, k)
	keyQuery := elastic.NewMatchQuery(keyField, v)
	return elastic.NewBoolQuery().Must(keyQuery)
}

func logErrorToSpan(span opentracing.Span, err error) {
	ottag.Error.Set(span, true)
	span.LogFields(otlog.Error(err))
}
