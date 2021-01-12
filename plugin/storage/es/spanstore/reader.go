// Copyright (c) 2019 The Jaeger Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/olivere/elastic"
	"github.com/opentracing/opentracing-go"
	ottag "github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	spanIndex               = "jaeger-span-"
	serviceIndex            = "jaeger-service-"
	archiveIndexSuffix      = "archive"
	archiveReadIndexSuffix  = archiveIndexSuffix + "-read"
	archiveWriteIndexSuffix = archiveIndexSuffix + "-write"
	traceIDAggregation      = "traceIDs"
	indexPrefixSeparator    = "-"

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

	defaultNumTraces = 100
)

var (
	// ErrServiceNameNotSet occurs when attempting to query with an empty service name
	ErrServiceNameNotSet = errors.New("service Name must be set")

	// ErrStartTimeMinGreaterThanMax occurs when start time min is above start time max
	ErrStartTimeMinGreaterThanMax = errors.New("start Time Minimum is above Maximum")

	// ErrDurationMinGreaterThanMax occurs when duration min is above duration max
	ErrDurationMinGreaterThanMax = errors.New("duration Minimum is above Maximum")

	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("malformed request object")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("start and End Time must be set")

	// ErrUnableToFindTraceIDAggregation occurs when an aggregation query for TraceIDs fail.
	ErrUnableToFindTraceIDAggregation = errors.New("could not find aggregation of traceIDs")

	defaultMaxDuration = model.DurationAsMicroseconds(time.Hour * 24)

	objectTagFieldList = []string{objectTagsField, objectProcessTagsField}

	nestedTagFieldList = []string{nestedTagsField, nestedProcessTagsField, nestedLogFieldsField}
)

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	client es.Client
	logger *zap.Logger
	// The age of the oldest service/operation we will look for. Because indices in ElasticSearch are by day,
	// this will be rounded down to UTC 00:00 of that day.
	maxSpanAge              time.Duration
	serviceOperationStorage *ServiceOperationStorage
	spanIndexPrefix         string
	serviceIndexPrefix      string
	indexDateLayout         string
	spanConverter           dbmodel.ToDomain
	timeRangeIndices        timeRangeIndexFn
	sourceFn                sourceFn
	maxDocCount             int
}

// SpanReaderParams holds constructor params for NewSpanReader
type SpanReaderParams struct {
	Client              es.Client
	Logger              *zap.Logger
	MaxSpanAge          time.Duration
	MaxDocCount         int
	MetricsFactory      metrics.Factory
	IndexPrefix         string
	IndexDateLayout     string
	TagDotReplacement   string
	Archive             bool
	UseReadWriteAliases bool
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(p SpanReaderParams) *SpanReader {
	return &SpanReader{
		client:                  p.Client,
		logger:                  p.Logger,
		maxSpanAge:              p.MaxSpanAge,
		serviceOperationStorage: NewServiceOperationStorage(p.Client, p.Logger, 0), // the decorator takes care of metrics
		spanIndexPrefix:         indexNames(p.IndexPrefix, spanIndex),
		serviceIndexPrefix:      indexNames(p.IndexPrefix, serviceIndex),
		indexDateLayout:         p.IndexDateLayout,
		spanConverter:           dbmodel.NewToDomain(p.TagDotReplacement),
		timeRangeIndices:        getTimeRangeIndexFn(p.Archive, p.UseReadWriteAliases),
		sourceFn:                getSourceFn(p.Archive, p.MaxDocCount),
		maxDocCount:             p.MaxDocCount,
	}
}

type timeRangeIndexFn func(indexName string, indexDateLayout string, startTime time.Time, endTime time.Time) []string

type sourceFn func(query elastic.Query, nextTime uint64) *elastic.SearchSource

func getTimeRangeIndexFn(archive, useReadWriteAliases bool) timeRangeIndexFn {
	if archive {
		var archivePrefix string
		if useReadWriteAliases {
			archivePrefix = archiveReadIndexSuffix
		} else {
			archivePrefix = archiveIndexSuffix
		}
		return func(indexName, indexDateLayout string, startTime time.Time, endTime time.Time) []string {
			return []string{archiveIndex(indexName, archivePrefix)}
		}
	}
	if useReadWriteAliases {
		return func(indices string, indexDateLayout string, startTime time.Time, endTime time.Time) []string {
			return []string{indices + "read"}
		}
	}
	return timeRangeIndices
}

func getSourceFn(archive bool, maxDocCount int) sourceFn {
	return func(query elastic.Query, nextTime uint64) *elastic.SearchSource {
		s := elastic.NewSearchSource().
			Query(query).
			Size(maxDocCount).
			TerminateAfter(maxDocCount)
		if !archive {
			s.Sort("startTime", true).
				SearchAfter(nextTime)
		}
		return s
	}
}

// timeRangeIndices returns the array of indices that we need to query, based on query params
func timeRangeIndices(indexName, indexDateLayout string, startTime time.Time, endTime time.Time) []string {
	var indices []string
	firstIndex := indexWithDate(indexName, indexDateLayout, startTime)
	currentIndex := indexWithDate(indexName, indexDateLayout, endTime)
	for currentIndex != firstIndex {
		indices = append(indices, currentIndex)
		endTime = endTime.Add(-24 * time.Hour)
		currentIndex = indexWithDate(indexName, indexDateLayout, endTime)
	}
	indices = append(indices, firstIndex)
	return indices
}

func indexNames(prefix, index string) string {
	if prefix != "" {
		return prefix + indexPrefixSeparator + index
	}
	return index
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetTrace")
	defer span.Finish()
	currentTime := time.Now()
	traces, err := s.multiRead(ctx, []model.TraceID{traceID}, currentTime.Add(-s.maxSpanAge), currentTime)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return traces[0], nil
}

func (s *SpanReader) collectSpans(esSpansRaw []*elastic.SearchHit) ([]*model.Span, error) {
	spans := make([]*model.Span, len(esSpansRaw))

	for i, esSpanRaw := range esSpansRaw {
		jsonSpan, err := s.unmarshalJSONSpan(esSpanRaw)
		if err != nil {
			return nil, fmt.Errorf("marshalling JSON to span object failed: %w", err)
		}
		span, err := s.spanConverter.SpanToDomain(jsonSpan)
		if err != nil {
			return nil, fmt.Errorf("converting JSONSpan to domain Span failed: %w", err)
		}
		spans[i] = span
	}
	return spans, nil
}

func (s *SpanReader) unmarshalJSONSpan(esSpanRaw *elastic.SearchHit) (*dbmodel.Span, error) {
	esSpanInByteArray := esSpanRaw.Source

	var jsonSpan dbmodel.Span

	d := json.NewDecoder(bytes.NewReader(*esSpanInByteArray))
	d.UseNumber()
	if err := d.Decode(&jsonSpan); err != nil {
		return nil, err
	}
	return &jsonSpan, nil
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetServices")
	defer span.Finish()
	currentTime := time.Now()
	jaegerIndices := s.timeRangeIndices(s.serviceIndexPrefix, s.indexDateLayout, currentTime.Add(-s.maxSpanAge), currentTime)
	return s.serviceOperationStorage.getServices(ctx, jaegerIndices, s.maxDocCount)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetOperations")
	defer span.Finish()
	currentTime := time.Now()
	jaegerIndices := s.timeRangeIndices(s.serviceIndexPrefix, s.indexDateLayout, currentTime.Add(-s.maxSpanAge), currentTime)
	operations, err := s.serviceOperationStorage.getOperations(ctx, jaegerIndices, query.ServiceName, s.maxDocCount)
	if err != nil {
		return nil, err
	}

	// TODO: https://github.com/jaegertracing/jaeger/issues/1923
	// 	- return the operations with actual span kind that meet requirement
	var result []spanstore.Operation
	for _, operation := range operations {
		result = append(result, spanstore.Operation{
			Name: operation,
		})
	}
	return result, err
}

func bucketToStringArray(buckets []*elastic.AggregationBucketKeyItem) ([]string, error) {
	strings := make([]string, len(buckets))
	for i, keyitem := range buckets {
		str, ok := keyitem.Key.(string)
		if !ok {
			return nil, errors.New("non-string key found in aggregation")
		}
		strings[i] = str
	}
	return strings, nil
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "FindTraces")
	defer span.Finish()

	uniqueTraceIDs, err := s.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	return s.multiRead(ctx, uniqueTraceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
}

// FindTraceIDs retrieves traces IDs that match the traceQuery
func (s *SpanReader) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "FindTraceIDs")
	defer span.Finish()

	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}

	esTraceIDs, err := s.findTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}

	return convertTraceIDsStringsToModels(esTraceIDs)
}

func (s *SpanReader) multiRead(ctx context.Context, traceIDs []model.TraceID, startTime, endTime time.Time) ([]*model.Trace, error) {

	childSpan, _ := opentracing.StartSpanFromContext(ctx, "multiRead")
	childSpan.LogFields(otlog.Object("trace_ids", traceIDs))
	defer childSpan.Finish()

	if len(traceIDs) == 0 {
		return []*model.Trace{}, nil
	}

	// Add an hour in both directions so that traces that straddle two indexes are retrieved.
	// i.e starts in one and ends in another.
	indices := s.timeRangeIndices(s.spanIndexPrefix, s.indexDateLayout, startTime.Add(-time.Hour), endTime.Add(time.Hour))
	nextTime := model.TimeAsEpochMicroseconds(startTime.Add(-time.Hour))

	searchAfterTime := make(map[model.TraceID]uint64)
	totalDocumentsFetched := make(map[model.TraceID]int)
	tracesMap := make(map[model.TraceID]*model.Trace)
	for {
		if len(traceIDs) == 0 {
			break
		}
		searchRequests := make([]*elastic.SearchRequest, len(traceIDs))
		for i, traceID := range traceIDs {
			query := buildTraceByIDQuery(traceID)
			if val, ok := searchAfterTime[traceID]; ok {
				nextTime = val
			}

			s := s.sourceFn(query, nextTime)

			searchRequests[i] = elastic.NewSearchRequest().
				IgnoreUnavailable(true).
				Source(s)
		}
		// set traceIDs to empty
		traceIDs = nil
		results, err := s.client.MultiSearch().Add(searchRequests...).Index(indices...).Do(ctx)

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

			if traceSpan, ok := tracesMap[lastSpan.TraceID]; ok {
				traceSpan.Spans = append(traceSpan.Spans, spans...)
			} else {
				tracesMap[lastSpan.TraceID] = &model.Trace{Spans: spans}
			}

			totalDocumentsFetched[lastSpan.TraceID] = totalDocumentsFetched[lastSpan.TraceID] + len(result.Hits.Hits)
			if totalDocumentsFetched[lastSpan.TraceID] < int(result.TotalHits()) {
				traceIDs = append(traceIDs, lastSpan.TraceID)
				searchAfterTime[lastSpan.TraceID] = model.TimeAsEpochMicroseconds(lastSpan.StartTime)
			}
		}
	}

	var traces []*model.Trace
	for _, trace := range tracesMap {
		traces = append(traces, trace)
	}
	return traces, nil
}

func buildTraceByIDQuery(traceID model.TraceID) elastic.Query {
	traceIDStr := traceID.String()
	if traceIDStr[0] != '0' {
		return elastic.NewTermQuery(traceIDField, traceIDStr)
	}
	// https://github.com/jaegertracing/jaeger/pull/1956 added leading zeros to IDs
	// So we need to also read IDs without leading zeros for compatibility with previously saved data.
	// TODO remove in newer versions, added in Jaeger 1.16
	var legacyTraceID string
	if traceID.High == 0 {
		legacyTraceID = fmt.Sprintf("%x", traceID.Low)
	} else {
		legacyTraceID = fmt.Sprintf("%x%016x", traceID.High, traceID.Low)
	}
	return elastic.NewBoolQuery().Should(
		elastic.NewTermQuery(traceIDField, traceIDStr).Boost(2),
		elastic.NewTermQuery(traceIDField, legacyTraceID))
}

func convertTraceIDsStringsToModels(traceIDs []string) ([]model.TraceID, error) {
	traceIDsMap := map[model.TraceID]bool{}
	// https://github.com/jaegertracing/jaeger/pull/1956 added leading zeros to IDs
	// So we need to also read IDs without leading zeros for compatibility with previously saved data.
	// That means the input to this function may contain logically identical trace IDs but formatted
	// with or without padding, and we need to dedupe them.
	// TODO remove deduping in newer versions, added in Jaeger 1.16
	traceIDsModels := make([]model.TraceID, 0, len(traceIDs))
	for _, ID := range traceIDs {
		traceID, err := model.TraceIDFromString(ID)
		if err != nil {
			return nil, fmt.Errorf("making traceID from string '%s' failed: %w", ID, err)
		}
		if _, ok := traceIDsMap[traceID]; !ok {
			traceIDsMap[traceID] = true
			traceIDsModels = append(traceIDsModels, traceID)
		}
	}

	return traceIDsModels, nil
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
	jaegerIndices := s.timeRangeIndices(s.spanIndexPrefix, s.indexDateLayout, traceQuery.StartTimeMin, traceQuery.StartTimeMax)

	searchService := s.client.Search(jaegerIndices...).
		Size(0). // set to 0 because we don't want actual documents.
		Aggregation(traceIDAggregation, aggregation).
		IgnoreUnavailable(true).
		Query(boolQuery)

	searchResult, err := searchService.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search services failed: %w", err)
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
	valueQuery := elastic.NewRegexpQuery(valueField, v)
	tagBoolQuery := elastic.NewBoolQuery().Must(keyQuery, valueQuery)
	return elastic.NewNestedQuery(field, tagBoolQuery)
}

func (s *SpanReader) buildObjectQuery(field string, k string, v string) elastic.Query {
	keyField := fmt.Sprintf("%s.%s", field, k)
	keyQuery := elastic.NewRegexpQuery(keyField, v)
	return elastic.NewBoolQuery().Must(keyQuery)
}

func logErrorToSpan(span opentracing.Span, err error) {
	ottag.Error.Set(span, true)
	span.LogFields(otlog.Error(err))
}
