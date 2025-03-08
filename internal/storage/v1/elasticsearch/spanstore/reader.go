// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/olivere/elastic"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	"github.com/jaegertracing/jaeger/pkg/es"
	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
)

const (
	spanIndexBaseName    = "jaeger-span-"
	serviceIndexBaseName = "jaeger-service-"
	traceIDAggregation   = "traceIDs"
	indexPrefixSeparator = "-"

	traceIDField           = "traceID"
	durationField          = "duration"
	startTimeField         = "startTime"
	startTimeMillisField   = "startTimeMillis"
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

	dawnOfTimeSpanAge = time.Hour * 24 * 365 * 50
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

// SpanReaderV1	is a wrapper around SpanReader
type SpanReaderV1 struct {
	spanReader    *SpanReader
	spanConverter dbmodel.ToDomain
	tracer        trace.Tracer
}

// NewSpanReaderV1 returns an instance of SpanReaderV1
func NewSpanReaderV1(p SpanReaderParams) *SpanReaderV1 {
	return &SpanReaderV1{
		spanReader:    NewSpanReader(p),
		spanConverter: dbmodel.NewToDomain(p.TagDotReplacement),
		tracer:        p.Tracer,
	}
}

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	client func() es.Client
	// The age of the oldest service/operation we will look for. Because indices in ElasticSearch are by day,
	// this will be rounded down to UTC 00:00 of that day.
	maxSpanAge              time.Duration
	serviceOperationStorage *ServiceOperationStorage
	spanIndexPrefix         string
	serviceIndexPrefix      string
	spanIndex               cfg.IndexOptions
	serviceIndex            cfg.IndexOptions
	timeRangeIndices        timeRangeIndexFn
	sourceFn                sourceFn
	maxDocCount             int
	useReadWriteAliases     bool
	logger                  *zap.Logger
	tracer                  trace.Tracer
	dotManager              dbmodel.DotReplacer
}

// SpanReaderParams holds constructor params for NewSpanReader
type SpanReaderParams struct {
	Client              func() es.Client
	MaxSpanAge          time.Duration
	MaxDocCount         int
	IndexPrefix         cfg.IndexPrefix
	SpanIndex           cfg.IndexOptions
	ServiceIndex        cfg.IndexOptions
	TagDotReplacement   string
	ReadAliasSuffix     string
	UseReadWriteAliases bool
	RemoteReadClusters  []string
	Logger              *zap.Logger
	Tracer              trace.Tracer
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(p SpanReaderParams) *SpanReader {
	maxSpanAge := p.MaxSpanAge
	readAliasSuffix := ""
	// Setting the maxSpanAge to a large duration will ensure all spans in the "read" alias are accessible by queries (query window = [now - maxSpanAge, now]).
	// When read/write aliases are enabled, which are required for index rollovers, only the "read" alias is queried and therefore should not affect performance.
	if p.UseReadWriteAliases {
		maxSpanAge = dawnOfTimeSpanAge
		if p.ReadAliasSuffix != "" {
			readAliasSuffix = p.ReadAliasSuffix
		} else {
			readAliasSuffix = "read"
		}
	}

	return &SpanReader{
		client:                  p.Client,
		maxSpanAge:              maxSpanAge,
		serviceOperationStorage: NewServiceOperationStorage(p.Client, p.Logger, 0), // the decorator takes care of metrics
		spanIndexPrefix:         p.IndexPrefix.Apply(spanIndexBaseName),
		serviceIndexPrefix:      p.IndexPrefix.Apply(serviceIndexBaseName),
		spanIndex:               p.SpanIndex,
		serviceIndex:            p.ServiceIndex,
		timeRangeIndices: getLoggingTimeRangeIndexFn(
			p.Logger,
			addRemoteReadClusters(
				getTimeRangeIndexFn(p.UseReadWriteAliases, readAliasSuffix),
				p.RemoteReadClusters,
			),
		),
		sourceFn:            getSourceFn(p.MaxDocCount),
		maxDocCount:         p.MaxDocCount,
		useReadWriteAliases: p.UseReadWriteAliases,
		logger:              p.Logger,
		tracer:              p.Tracer,
		dotManager:          dbmodel.NewDotReplacer(p.TagDotReplacement),
	}
}

type timeRangeIndexFn func(indexName string, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string

type sourceFn func(query elastic.Query, nextTime uint64) *elastic.SearchSource

func getLoggingTimeRangeIndexFn(logger *zap.Logger, fn timeRangeIndexFn) timeRangeIndexFn {
	if !logger.Core().Enabled(zap.DebugLevel) {
		return fn
	}
	return func(indexName string, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
		indices := fn(indexName, indexDateLayout, startTime, endTime, reduceDuration)
		logger.Debug("Reading from ES indices", zap.Strings("index", indices))
		return indices
	}
}

func getTimeRangeIndexFn(useReadWriteAliases bool, readAlias string) timeRangeIndexFn {
	if useReadWriteAliases {
		return func(indexPrefix, _ /* indexDateLayout */ string, _ /* startTime */ time.Time, _ /* endTime */ time.Time, _ /* reduceDuration */ time.Duration) []string {
			return []string{indexPrefix + readAlias}
		}
	}
	return timeRangeIndices
}

// Add a remote cluster prefix for each cluster and for each index and add it to the list of original indices.
// Elasticsearch cross cluster api example GET /twitter,cluster_one:twitter,cluster_two:twitter/_search.
func addRemoteReadClusters(fn timeRangeIndexFn, remoteReadClusters []string) timeRangeIndexFn {
	return func(indexPrefix string, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
		jaegerIndices := fn(indexPrefix, indexDateLayout, startTime, endTime, reduceDuration)
		if len(remoteReadClusters) == 0 {
			return jaegerIndices
		}

		for _, jaegerIndex := range jaegerIndices {
			for _, remoteCluster := range remoteReadClusters {
				remoteIndex := remoteCluster + ":" + jaegerIndex
				jaegerIndices = append(jaegerIndices, remoteIndex)
			}
		}

		return jaegerIndices
	}
}

func getSourceFn(maxDocCount int) sourceFn {
	return func(query elastic.Query, nextTime uint64) *elastic.SearchSource {
		return elastic.NewSearchSource().
			Query(query).
			Size(maxDocCount).
			Sort("startTime", true).
			SearchAfter(nextTime)
	}
}

// timeRangeIndices returns the array of indices that we need to query, based on query params
func timeRangeIndices(indexName, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
	var indices []string
	firstIndex := indexWithDate(indexName, indexDateLayout, startTime)
	currentIndex := indexWithDate(indexName, indexDateLayout, endTime)
	for currentIndex != firstIndex {
		if len(indices) == 0 || indices[len(indices)-1] != currentIndex {
			indices = append(indices, currentIndex)
		}
		endTime = endTime.Add(reduceDuration)
		currentIndex = indexWithDate(indexName, indexDateLayout, endTime)
	}
	indices = append(indices, firstIndex)
	return indices
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReaderV1) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	ctx, span := s.spanReader.tracer.Start(ctx, "GetTrace")
	defer span.End()
	currentTime := time.Now()
	// TODO: use start time & end time in "query" struct
	traces, err := s.multiRead(ctx, []model.TraceID{query.TraceID}, currentTime.Add(-s.spanReader.maxSpanAge), currentTime)
	if err != nil {
		return nil, es.DetailedError(err)
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return traces[0], nil
}

func (s *SpanReader) collectJsonSpans(esSpansRaw []*elastic.SearchHit) ([]*dbmodel.Span, error) {
	spans := make([]*dbmodel.Span, len(esSpansRaw))
	for i, esSpanRaw := range esSpansRaw {
		jsonSpan, err := s.unmarshalJSONSpan(esSpanRaw)
		if err != nil {
			return nil, fmt.Errorf("marshalling JSON to span object failed: %w", err)
		}
		spans[i] = jsonSpan
	}
	return spans, nil
}

func (s *SpanReaderV1) collectSpans(jsonSpans []*dbmodel.Span) ([]*model.Span, error) {
	spans := make([]*model.Span, len(jsonSpans))

	for i, jsonSpan := range jsonSpans {
		span, err := s.spanConverter.SpanToDomain(jsonSpan)
		if err != nil {
			return nil, fmt.Errorf("converting JSONSpan to domain Span failed: %w", err)
		}
		spans[i] = span
	}
	return spans, nil
}

func (*SpanReader) unmarshalJSONSpan(esSpanRaw *elastic.SearchHit) (*dbmodel.Span, error) {
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
func (s *SpanReaderV1) GetServices(ctx context.Context) ([]string, error) {
	return s.spanReader.GetServices(ctx)
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	ctx, span := s.tracer.Start(ctx, "GetService")
	defer span.End()
	currentTime := time.Now()
	jaegerIndices := s.timeRangeIndices(
		s.serviceIndexPrefix,
		s.serviceIndex.DateLayout,
		currentTime.Add(-s.maxSpanAge),
		currentTime,
		cfg.RolloverFrequencyAsNegativeDuration(s.serviceIndex.RolloverFrequency),
	)
	return s.serviceOperationStorage.getServices(ctx, jaegerIndices, s.maxDocCount)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReaderV1) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	operations, err := s.spanReader.GetOperations(ctx, esOperationQueryParamsFromSpanStoreQueryParams(query))
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/jaegertracing/jaeger/issues/1923
	// 	- return the operations with actual span kind that meet requirement
	var result []spanstore.Operation
	for _, operation := range operations {
		result = append(result, spanstore.Operation{
			Name: operation.Name,
		})
	}
	return result, err
}

func esOperationQueryParamsFromSpanStoreQueryParams(p spanstore.OperationQueryParameters) dbmodel.OperationQueryParameters {
	return dbmodel.OperationQueryParameters{
		ServiceName: p.ServiceName,
		SpanKind:    p.SpanKind,
	}
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(
	ctx context.Context,
	parameters dbmodel.OperationQueryParameters,
) ([]dbmodel.Operation, error) {
	ctx, span := s.tracer.Start(ctx, "GetOperations")
	defer span.End()
	currentTime := time.Now()
	jaegerIndices := s.timeRangeIndices(
		s.serviceIndexPrefix,
		s.serviceIndex.DateLayout,
		currentTime.Add(-s.maxSpanAge),
		currentTime,
		cfg.RolloverFrequencyAsNegativeDuration(s.serviceIndex.RolloverFrequency),
	)
	var result []dbmodel.Operation
	operations, err := s.serviceOperationStorage.getOperations(ctx, jaegerIndices, parameters.ServiceName, s.maxDocCount)
	if err != nil {
		return nil, err
	}
	for _, operation := range operations {
		result = append(result, dbmodel.Operation{Name: operation})
	}
	return result, nil
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
func (s *SpanReaderV1) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	ctx, span := s.spanReader.tracer.Start(ctx, "FindTraces")
	defer span.End()

	uniqueTraceIDs, err := s.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, es.DetailedError(err)
	}
	return s.multiRead(ctx, uniqueTraceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
}

// FindTraceIDs retrieves traces IDs that match the traceQuery
func (s *SpanReaderV1) FindTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	ctx, span := s.spanReader.tracer.Start(ctx, "FindTraceIDs")
	defer span.End()

	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.NumTraces == 0 {
		traceQuery.NumTraces = defaultNumTraces
	}

	esTraceIDs, err := s.spanReader.FindTraceIDs(ctx, esTraceQueryParamsFromSpanStoreTraceQueryParams(traceQuery))
	if err != nil {
		return nil, err
	}

	return convertTraceIDsStringsToModels(esTraceIDs)
}

func esTraceQueryParamsFromSpanStoreTraceQueryParams(p *spanstore.TraceQueryParameters) *dbmodel.TraceQueryParameters {
	return &dbmodel.TraceQueryParameters{
		ServiceName:   p.ServiceName,
		OperationName: p.OperationName,
		Tags:          p.Tags,
		StartTimeMin:  p.StartTimeMin,
		StartTimeMax:  p.StartTimeMax,
		DurationMin:   p.DurationMin,
		DurationMax:   p.DurationMax,
		NumTraces:     p.NumTraces,
	}
}

func (s *SpanReaderV1) multiRead(ctx context.Context, traceIDs []model.TraceID, startTime, endTime time.Time) ([]*model.Trace, error) {
	ctx, childSpan := s.tracer.Start(ctx, "multiRead")
	defer childSpan.End()
	traceIds := make(map[dbmodel.TraceID]string)
	for _, traceID := range traceIDs {
		traceIds[dbmodel.TraceID(traceID.String())] = getLegacyTraceId(traceID)
	}
	tracesMap, err := s.spanReader.MultiRead(ctx, traceIds, startTime, endTime)
	if err != nil {
		return []*model.Trace{}, err
	}
	var traces []*model.Trace
	for _, jsonSpans := range tracesMap {
		spans, err := s.collectSpans(jsonSpans)
		if err != nil {
			err = es.DetailedError(err)
			logErrorToSpan(childSpan, err)
			return nil, err
		}
		traces = append(traces, &model.Trace{Spans: spans})
	}
	return traces, nil
}

func getLegacyTraceId(traceID model.TraceID) string {
	// https://github.com/jaegertracing/jaeger/pull/1956 added leading zeros to IDs
	// So we need to also read IDs without leading zeros for compatibility with previously saved data.
	// TODO remove in newer versions, added in Jaeger 1.16
	if traceID.High == 0 {
		return strconv.FormatUint(traceID.Low, 16)
	}
	return fmt.Sprintf("%x%016x", traceID.High, traceID.Low)
}

func (s *SpanReader) MultiRead(ctx context.Context, traceIds map[dbmodel.TraceID]string, startTime, endTime time.Time) (map[dbmodel.TraceID][]*dbmodel.Span, error) {
	ctx, childSpan := s.tracer.Start(ctx, "MultiRead")
	defer childSpan.End()

	var traceIdSlice []dbmodel.TraceID
	for traceID := range traceIds {
		traceIdSlice = append(traceIdSlice, traceID)
	}

	if childSpan.IsRecording() {
		tracesIDs := make([]string, len(traceIdSlice))
		for i, traceID := range traceIdSlice {
			tracesIDs[i] = string(traceID)
		}
		childSpan.SetAttributes(attribute.Key("trace_ids").StringSlice(tracesIDs))
	}
	tracesMap := make(map[dbmodel.TraceID][]*dbmodel.Span)
	if len(traceIds) == 0 {
		return tracesMap, nil
	}

	// Add an hour in both directions so that traces that straddle two indexes are retrieved.
	// i.e starts in one and ends in another.
	indices := s.timeRangeIndices(
		s.spanIndexPrefix,
		s.spanIndex.DateLayout,
		startTime.Add(-time.Hour),
		endTime.Add(time.Hour),
		cfg.RolloverFrequencyAsNegativeDuration(s.spanIndex.RolloverFrequency),
	)
	nextTime := model.TimeAsEpochMicroseconds(startTime.Add(-time.Hour))
	searchAfterTime := make(map[dbmodel.TraceID]uint64)
	totalDocumentsFetched := make(map[dbmodel.TraceID]int)
	for {
		if len(traceIdSlice) == 0 {
			break
		}
		searchRequests := make([]*elastic.SearchRequest, len(traceIdSlice))
		for i, traceID := range traceIdSlice {
			legacyTraceID := traceIds[traceID]
			traceQuery := buildTraceByIDQuery(traceID, legacyTraceID)
			query := elastic.NewBoolQuery().
				Must(traceQuery)
			if s.useReadWriteAliases {
				startTimeRangeQuery := s.buildStartTimeQuery(startTime.Add(-time.Hour*24), endTime.Add(time.Hour*24))
				query = query.Must(startTimeRangeQuery)
			}

			if val, ok := searchAfterTime[traceID]; ok {
				nextTime = val
			}

			s := s.sourceFn(query, nextTime)
			searchRequests[i] = elastic.NewSearchRequest().
				IgnoreUnavailable(true).
				Source(s)
		}
		// set traceIDs to empty
		traceIdSlice = nil
		results, err := s.client().MultiSearch().Add(searchRequests...).Index(indices...).Do(ctx)
		if err != nil {
			err = es.DetailedError(err)
			logErrorToSpan(childSpan, err)
			return nil, err
		}

		if len(results.Responses) == 0 {
			break
		}

		for _, result := range results.Responses {
			if result.Hits == nil || len(result.Hits.Hits) == 0 {
				continue
			}
			spans, err := s.collectJsonSpans(result.Hits.Hits)
			if err != nil {
				err = es.DetailedError(err)
				logErrorToSpan(childSpan, err)
				return nil, err
			}
			lastSpan := spans[len(spans)-1]

			if traceSpan, ok := tracesMap[lastSpan.TraceID]; ok {
				traceSpan = append(traceSpan, spans...)
				tracesMap[lastSpan.TraceID] = traceSpan
			} else {
				tracesMap[lastSpan.TraceID] = spans
			}
			totalDocumentsFetched[lastSpan.TraceID] += len(result.Hits.Hits)
			if totalDocumentsFetched[lastSpan.TraceID] < int(result.TotalHits()) {
				traceIdSlice = append(traceIdSlice, lastSpan.TraceID)
				searchAfterTime[lastSpan.TraceID] = lastSpan.StartTime
			}
		}
	}
	return tracesMap, nil
}

func buildTraceByIDQuery(traceID dbmodel.TraceID, legacyTraceID string) elastic.Query {
	traceIDStr := string(traceID)
	if traceIDStr[0] != '0' {
		return elastic.NewTermQuery(traceIDField, traceIDStr)
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

func (s *SpanReader) FindTraceIDs(
	ctx context.Context,
	p *dbmodel.TraceQueryParameters,
) ([]string, error) {
	ctx, childSpan := s.tracer.Start(ctx, "findTraceIDs")
	defer childSpan.End()
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
	aggregation := s.buildTraceIDAggregation(p.NumTraces)
	boolQuery := s.buildFindTraceIDsQuery(p)
	jaegerIndices := s.timeRangeIndices(
		s.spanIndexPrefix,
		s.spanIndex.DateLayout,
		p.StartTimeMin,
		p.StartTimeMax,
		cfg.RolloverFrequencyAsNegativeDuration(s.spanIndex.RolloverFrequency),
	)

	searchService := s.client().Search(jaegerIndices...).
		Size(0). // set to 0 because we don't want actual documents.
		Aggregation(traceIDAggregation, aggregation).
		IgnoreUnavailable(true).
		Query(boolQuery)

	searchResult, err := searchService.Do(ctx)
	if err != nil {
		err = es.DetailedError(err)
		s.logger.Info("es search services failed", zap.Any("traceQuery", serviceName), zap.Error(err))
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

func (*SpanReader) buildTraceIDSubAggregation() elastic.Aggregation {
	return elastic.NewMaxAggregation().
		Field(startTimeField)
}

func (s *SpanReader) buildFindTraceIDsQuery(p *dbmodel.TraceQueryParameters) elastic.Query {
	boolQuery := elastic.NewBoolQuery()

	// add duration query
	if p.DurationMax != 0 || p.DurationMin != 0 {
		durationQuery := s.buildDurationQuery(p.DurationMin, p.DurationMax)
		boolQuery.Must(durationQuery)
	}

	// add startTime query
	startTimeQuery := s.buildStartTimeQuery(p.StartTimeMin, p.StartTimeMax)
	boolQuery.Must(startTimeQuery)

	// add process.serviceName query
	if p.ServiceName != "" {
		serviceNameQuery := s.buildServiceNameQuery(p.ServiceName)
		boolQuery.Must(serviceNameQuery)
	}

	// add operationName query
	if p.OperationName != "" {
		operationNameQuery := s.buildOperationNameQuery(p.OperationName)
		boolQuery.Must(operationNameQuery)
	}

	for k, v := range p.Tags {
		tagQuery := s.buildTagQuery(k, v)
		boolQuery.Must(tagQuery)
	}
	return boolQuery
}

func (*SpanReader) buildDurationQuery(durationMin time.Duration, durationMax time.Duration) elastic.Query {
	minDurationMicros := model.DurationAsMicroseconds(durationMin)
	maxDurationMicros := defaultMaxDuration
	if durationMax != 0 {
		maxDurationMicros = model.DurationAsMicroseconds(durationMax)
	}
	return elastic.NewRangeQuery(durationField).Gte(minDurationMicros).Lte(maxDurationMicros)
}

func (*SpanReader) buildStartTimeQuery(startTimeMin time.Time, startTimeMax time.Time) elastic.Query {
	minStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMin)
	maxStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMax)
	// startTimeMillisField is date field in ES mapping.
	// Using date field in range queries helps to skip search on unnecessary shards at Elasticsearch side.
	// https://discuss.elastic.co/t/timeline-query-on-timestamped-indices/129328/2
	return elastic.NewRangeQuery(startTimeMillisField).Gte(minStartTimeMicros / 1000).Lte(maxStartTimeMicros / 1000)
}

func (*SpanReader) buildServiceNameQuery(serviceName string) elastic.Query {
	return elastic.NewMatchQuery(serviceNameField, serviceName)
}

func (*SpanReader) buildOperationNameQuery(operationName string) elastic.Query {
	return elastic.NewMatchQuery(operationNameField, operationName)
}

func (s *SpanReader) buildTagQuery(k string, v string) elastic.Query {
	objectTagListLen := len(objectTagFieldList)
	queries := make([]elastic.Query, len(nestedTagFieldList)+objectTagListLen)
	kd := s.dotManager.ReplaceDot(k)
	for i := range objectTagFieldList {
		queries[i] = s.buildObjectQuery(objectTagFieldList[i], kd, v)
	}
	for i := range nestedTagFieldList {
		queries[i+objectTagListLen] = s.buildNestedQuery(nestedTagFieldList[i], k, v)
	}

	// but configuration can change over time
	return elastic.NewBoolQuery().Should(queries...)
}

func (*SpanReader) buildNestedQuery(field string, k string, v string) elastic.Query {
	keyField := fmt.Sprintf("%s.%s", field, tagKeyField)
	valueField := fmt.Sprintf("%s.%s", field, tagValueField)
	keyQuery := elastic.NewMatchQuery(keyField, k)
	valueQuery := elastic.NewRegexpQuery(valueField, v)
	tagBoolQuery := elastic.NewBoolQuery().Must(keyQuery, valueQuery)
	return elastic.NewNestedQuery(field, tagBoolQuery)
}

func (*SpanReader) buildObjectQuery(field string, k string, v string) elastic.Query {
	keyField := fmt.Sprintf("%s.%s", field, k)
	keyQuery := elastic.NewRegexpQuery(keyField, v)
	return elastic.NewBoolQuery().Must(keyQuery)
}

func logErrorToSpan(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
