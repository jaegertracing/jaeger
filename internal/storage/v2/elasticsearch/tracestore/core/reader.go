// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const (
	traceIDAggregation   = "traceIDs"
	indexPrefixSeparator = "-"

	traceIDField           = "traceID"
	durationField          = "duration"
	startTimeField         = "startTime"
	startTimeMillisField   = "startTimeMillis"
	serviceNameField       = "process.serviceName"
	operationNameField     = "operationName"
	parentSpanIDField      = "parentSpanID"
	objectTagsField        = "tag"
	objectProcessTagsField = "process.tag"
	nestedTagsField        = "tags"
	nestedProcessTagsField = "process.tags"
	nestedLogFieldsField   = "logs.fields"
	tagKeyField            = "key"
	tagValueField          = "value"

	defaultSearchDepth = 100

	DawnOfTimeSpanAge = time.Hour * 24 * 365 * 50
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

	_ Reader = (*SpanReader)(nil) // check API conformance

	disableLegacyIDs *featuregate.Gate
)

func init() {
	disableLegacyIDs = featuregate.GlobalRegistry().MustRegister(
		"jaeger.es.disableLegacyId",
		featuregate.StageStable, // enabled by default and cannot be disabled
		featuregate.WithRegisterFromVersion("v2.5.0"),
		featuregate.WithRegisterToVersion("v2.8.0"),
		featuregate.WithRegisterDescription("Legacy trace ids are the ids that used to be rendered with leading 0s omitted. Setting this gate to false will force the reader to search for the spans with trace ids having leading zeroes"),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/1578"),
	)
}

// Time-range design (referenced as "timeRangeDesign" in comments below):
//
// There are two read operations with different time-range semantics:
//
//  1. FindTraceIDs: the user always supplies [StartTimeMin, StartTimeMax]. No adjustment needed.
//
//  2. GetTraces (by trace ID): no time range is known. The reader uses [now-maxSpanAge, now].
//     - Periodic indices: maxSpanAge should match data retention (e.g., 7d). ReadTargets generates
//       only existing indices within that window.
//     - Alias indices: a single alias covers all data. The factory overrides maxSpanAge to
//       DawnOfTimeSpanAge (50 years) so the time-range filter doesn't exclude old traces.
//
// multiRead always adds a time-range filter to the ES query for shard pruning (helps ES skip
// irrelevant shards). This is harmless for periodic indices and essential for aliases.
//
// multiRead is called from both paths. When called from FindTraces, the time range is the user's
// search window — but a trace may extend beyond it. The ±maxTraceDuration padding on index
// selection and the ES time filter ensure all spans of a trace are found even when they extend
// beyond the search window.
//
// TODO: future improvement:
//   - With data streams, store a materialized view of (traceID, minStartTime, maxEndTime) to
//     enable precise time-range scoping for GetTraces (similar to ClickHouse approach).

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	searcher esclient.Searcher
	// The age of the oldest service/operation we will look for. Because indices in ElasticSearch are by day,
	// this will be rounded down to UTC 00:00 of that day.
	maxSpanAge              time.Duration
	maxTraceDuration        time.Duration
	serviceOperationStorage *ServiceOperationStorage
	spanRotation            indices.Rotation
	serviceRotation         indices.Rotation
	maxDocCount             int
	logger                  *zap.Logger
	tracer                  trace.Tracer
	dotReplacer             dbmodel.DotReplacer
}

// SpanReaderParams holds constructor params for NewSpanReader
type SpanReaderParams struct {
	// Searcher is the esclient data-plane search client backing every read path:
	// service/operation reads, trace-ID and trace lookups, and native summaries.
	Searcher          esclient.Searcher
	MaxSpanAge        time.Duration
	MaxTraceDuration  time.Duration
	MaxDocCount       int
	TagDotReplacement string
	Logger            *zap.Logger
	Tracer            trace.Tracer
	SpanRotation      indices.Rotation
	ServiceRotation   indices.Rotation
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(p SpanReaderParams) *SpanReader {
	return &SpanReader{
		searcher:                p.Searcher,
		maxSpanAge:              p.MaxSpanAge,
		maxTraceDuration:        p.MaxTraceDuration,
		serviceOperationStorage: NewServiceOperationStorage(p.Searcher, nil, p.Logger, 0), // read-only: no bulk writer; the decorator takes care of metrics
		spanRotation:            p.SpanRotation,
		serviceRotation:         p.ServiceRotation,
		maxDocCount:             p.MaxDocCount,
		logger:                  p.Logger,
		tracer:                  p.Tracer,
		dotReplacer:             dbmodel.NewDotReplacer(p.TagDotReplacement),
	}
}

// buildTraceReadRequest builds the per-trace search body multiRead pages through:
// the trace's query, ordered by startTime ascending, with search_after for the
// pagination cursor and track_total_hits so the loop knows when a trace is fully
// fetched.
func (s *SpanReader) buildTraceReadRequest(q esquery.Query, nextTime uint64) esclient.SearchRequest {
	return esclient.SearchRequest{
		Query:          q,
		Size:           s.maxDocCount,
		Sort:           []esclient.SortOrder{{Field: startTimeField, Order: "asc"}},
		SearchAfter:    []any{nextTime},
		TrackTotalHits: true,
	}
}

// GetTraces takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTraces(ctx context.Context, query []dbmodel.TraceID) ([]dbmodel.Trace, error) {
	ctx, span := s.tracer.Start(ctx, "GetTrace")
	defer span.End()
	currentTime := time.Now()
	// TODO: use start time & end time in "query" struct
	return s.multiRead(ctx, query, currentTime.Add(-s.maxSpanAge), currentTime)
}

func (s *SpanReader) collectSpans(esSpansRaw []esclient.SearchHit) ([]dbmodel.Span, error) {
	spans := make([]dbmodel.Span, len(esSpansRaw))

	for i, esSpanRaw := range esSpansRaw {
		dbSpan, err := s.unmarshalJSONSpan(esSpanRaw)
		if err != nil {
			return nil, fmt.Errorf("marshalling JSON to span object failed: %w", err)
		}
		s.mergeAllNestedAndElevatedTagsOfSpan(&dbSpan)
		spans[i] = dbSpan
	}
	return spans, nil
}

func (*SpanReader) unmarshalJSONSpan(esSpanRaw esclient.SearchHit) (dbmodel.Span, error) {
	esSpanInByteArray := esSpanRaw.Source

	var jsonSpan dbmodel.Span

	d := json.NewDecoder(bytes.NewReader(esSpanInByteArray))
	d.UseNumber()
	if err := d.Decode(&jsonSpan); err != nil {
		return dbmodel.Span{}, err
	}
	return jsonSpan, nil
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	ctx, span := s.tracer.Start(ctx, "GetService")
	defer span.End()
	currentTime := time.Now()
	jaegerIndices := s.serviceRotation.ReadTargets(currentTime.Add(-s.maxSpanAge), currentTime)
	return s.serviceOperationStorage.getServices(ctx, jaegerIndices, s.maxDocCount)
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(
	ctx context.Context,
	query dbmodel.OperationQueryParameters,
) ([]dbmodel.Operation, error) {
	ctx, span := s.tracer.Start(ctx, "GetOperations")
	defer span.End()
	currentTime := time.Now()
	jaegerIndices := s.serviceRotation.ReadTargets(currentTime.Add(-s.maxSpanAge), currentTime)
	operations, err := s.serviceOperationStorage.getOperations(ctx, jaegerIndices, query.ServiceName, s.maxDocCount)
	if err != nil {
		return nil, err
	}

	// TODO: https://github.com/jaegertracing/jaeger/issues/1923
	// 	- return the operations with actual span kind that meet requirement
	var result []dbmodel.Operation
	for _, operation := range operations {
		result = append(result, dbmodel.Operation{
			Name: operation,
		})
	}
	return result, err
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(ctx context.Context, traceQuery dbmodel.TraceQueryParameters) ([]dbmodel.Trace, error) {
	ctx, span := s.tracer.Start(ctx, "FindTraces")
	defer span.End()

	uniqueTraceIDs, err := s.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, es.DetailedError(err)
	}
	return s.multiRead(ctx, uniqueTraceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
}

// FindTraceIDs retrieves traces IDs that match the traceQuery
func (s *SpanReader) FindTraceIDs(ctx context.Context, traceQuery dbmodel.TraceQueryParameters) ([]dbmodel.TraceID, error) {
	ctx, span := s.tracer.Start(ctx, "FindTraceIDs")
	defer span.End()

	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.SearchDepth == 0 {
		traceQuery.SearchDepth = defaultSearchDepth
	}

	esTraceIDs, err := s.findTraceIDsFromQuery(ctx, traceQuery)
	if err != nil {
		return nil, err
	}

	return esTraceIDs, nil
}

func (s *SpanReader) multiRead(ctx context.Context, traceIDs []dbmodel.TraceID, startTime, endTime time.Time) ([]dbmodel.Trace, error) {
	ctx, childSpan := s.tracer.Start(ctx, "multiRead")
	defer childSpan.End()

	if childSpan.IsRecording() {
		tracesIDs := make([]string, len(traceIDs))
		for i, traceID := range traceIDs {
			tracesIDs[i] = string(traceID)
		}
		childSpan.SetAttributes(attribute.Key("trace_ids").StringSlice(tracesIDs))
	}

	traces := make([]dbmodel.Trace, 0, len(traceIDs))

	if len(traceIDs) == 0 {
		return traces, nil
	}

	// See timeRangeDesign above for context on the padding and the alias filter.
	idxList := s.spanRotation.ReadTargets(startTime.Add(-s.maxTraceDuration), endTime.Add(s.maxTraceDuration))
	nextTime := model.TimeAsEpochMicroseconds(startTime.Add(-s.maxTraceDuration))
	searchAfterTime := make(map[dbmodel.TraceID]uint64)
	totalDocumentsFetched := make(map[dbmodel.TraceID]int)
	tracesMap := make(map[dbmodel.TraceID]*dbmodel.Trace)
	for len(traceIDs) != 0 {
		searchRequests := make([]esclient.MultiSearchRequest, len(traceIDs))
		for i, traceID := range traceIDs {
			traceQuery := buildTraceByIDQuery(traceID)
			startTimeRangeQuery := s.buildStartTimeQuery(startTime.Add(-s.maxTraceDuration), endTime.Add(s.maxTraceDuration))
			query := esquery.NewBoolQuery().
				Must(traceQuery).
				Must(startTimeRangeQuery)

			if val, ok := searchAfterTime[traceID]; ok {
				nextTime = val
			}

			searchRequests[i] = esclient.MultiSearchRequest{
				Indices: idxList,
				Search:  s.buildTraceReadRequest(query, nextTime),
			}
		}
		// set traceIDs to empty
		traceIDs = nil
		responses, err := s.searcher.MultiSearch(ctx, searchRequests)
		if err != nil {
			err = es.DetailedError(err)
			logErrorToSpan(childSpan, err)
			return nil, err
		}

		if len(responses) == 0 {
			break
		}

		for _, result := range responses {
			if len(result.Hits.Hits) == 0 {
				continue
			}
			spans, err := s.collectSpans(result.Hits.Hits)
			if err != nil {
				err = es.DetailedError(err)
				logErrorToSpan(childSpan, err)
				return nil, err
			}
			lastSpan := spans[len(spans)-1]

			if traceSpan, ok := tracesMap[lastSpan.TraceID]; ok {
				traceSpan.Spans = append(traceSpan.Spans, spans...)
			} else {
				traces = append(traces, dbmodel.Trace{Spans: spans})
				tracesMap[lastSpan.TraceID] = &traces[len(traces)-1]
			}

			totalDocumentsFetched[lastSpan.TraceID] += len(result.Hits.Hits)
			if totalDocumentsFetched[lastSpan.TraceID] < result.Hits.Total.Value {
				traceIDs = append(traceIDs, lastSpan.TraceID)
				searchAfterTime[lastSpan.TraceID] = lastSpan.StartTime
			}
		}
	}
	return traces, nil
}

func buildTraceByIDQuery(traceID dbmodel.TraceID) esquery.Query {
	return buildTraceByIDQueryWithLegacy(traceID, disableLegacyIDs.IsEnabled())
}

// buildTraceByIDQueryWithLegacy takes the gate value as a parameter so the
// legacy-ID branch — unreachable in production while the stable
// jaeger.es.disableLegacyId gate is enabled — stays testable.
func buildTraceByIDQueryWithLegacy(traceID dbmodel.TraceID, disableLegacy bool) esquery.Query {
	traceIDStr := string(traceID)
	if traceIDStr[0] != '0' || disableLegacy {
		return esquery.NewTermQuery(traceIDField, traceIDStr)
	}
	// https://github.com/jaegertracing/jaeger/pull/1956 added leading zeros to IDs
	// So we need to also read IDs without leading zeros for compatibility with previously saved data.
	legacyTraceID := strings.TrimLeft(traceIDStr, "0")
	return esquery.NewBoolQuery().Should(
		esquery.NewTermQuery(traceIDField, traceIDStr).Boost(2),
		esquery.NewTermQuery(traceIDField, legacyTraceID),
	)
}

func validateQuery(p dbmodel.TraceQueryParameters) error {
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

func (s *SpanReader) findTraceIDsFromQuery(ctx context.Context, traceQuery dbmodel.TraceQueryParameters) ([]dbmodel.TraceID, error) {
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
	aggregation := s.buildTraceIDAggregation(traceQuery.SearchDepth)
	boolQuery := s.buildFindTraceIDsQuery(traceQuery)
	jaegerIndices := s.spanRotation.ReadTargets(traceQuery.StartTimeMin, traceQuery.StartTimeMax)

	searchResult, err := s.searcher.Search(ctx, jaegerIndices, esclient.SearchRequest{
		Size:  0, // set to 0 because we don't want actual documents.
		Query: boolQuery,
		Aggregations: map[string]esquery.Aggregation{
			traceIDAggregation: aggregation,
		},
	})
	if err != nil {
		err = es.DetailedError(err)
		s.logger.Info("es search services failed", zap.Any("traceQuery", traceQuery), zap.Error(err))
		return nil, fmt.Errorf("search services failed: %w", err)
	}
	if searchResult.Aggregations == nil {
		return []dbmodel.TraceID{}, nil
	}
	bucket, found := searchResult.Aggregations[traceIDAggregation]
	if !found {
		return nil, ErrUnableToFindTraceIDAggregation
	}

	traceIDs := make([]dbmodel.TraceID, len(bucket.Buckets))
	for i, b := range bucket.Buckets {
		traceIDs[i] = dbmodel.TraceID(b.Key)
	}
	return traceIDs, nil
}

func (s *SpanReader) buildTraceIDAggregation(numOfTraces int) esquery.Aggregation {
	return esquery.NewTermsAggregation(traceIDField).
		Size(numOfTraces).
		Order(startTimeField, "desc").
		SubAggregation(startTimeField, s.buildTraceIDSubAggregation())
}

func (*SpanReader) buildTraceIDSubAggregation() esquery.Aggregation {
	return esquery.NewMaxAggregation(startTimeField)
}

func (s *SpanReader) buildFindTraceIDsQuery(traceQuery dbmodel.TraceQueryParameters) esquery.Query {
	boolQuery := esquery.NewBoolQuery()

	// add duration query
	if traceQuery.DurationMax != 0 || traceQuery.DurationMin != 0 {
		durationQuery := s.buildDurationQuery(traceQuery.DurationMin, traceQuery.DurationMax)
		boolQuery.Must(durationQuery)
	}

	// add startTime query
	startTimeQuery := s.buildStartTimeQuery(traceQuery.StartTimeMin, traceQuery.StartTimeMax)
	boolQuery.Must(startTimeQuery)

	// add process.serviceName query
	if traceQuery.ServiceName != "" {
		serviceNameQuery := s.buildServiceNameQuery(traceQuery.ServiceName)
		boolQuery.Must(serviceNameQuery)
	}

	// add operationName query
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

func (*SpanReader) buildDurationQuery(durationMin time.Duration, durationMax time.Duration) esquery.Query {
	minDurationMicros := model.DurationAsMicroseconds(durationMin)
	maxDurationMicros := defaultMaxDuration
	if durationMax != 0 {
		maxDurationMicros = model.DurationAsMicroseconds(durationMax)
	}
	return esquery.NewRangeQuery(durationField).Gte(minDurationMicros).Lte(maxDurationMicros)
}

func (*SpanReader) buildStartTimeQuery(startTimeMin time.Time, startTimeMax time.Time) esquery.Query {
	minStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMin)
	maxStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMax)
	// startTimeMillisField is date field in ES mapping.
	// Using date field in range queries helps to skip search on unnecessary shards at Elasticsearch side.
	// https://discuss.elastic.co/t/timeline-query-on-timestamped-indices/129328/2
	return esquery.NewRangeQuery(startTimeMillisField).Gte(minStartTimeMicros / 1000).Lte(maxStartTimeMicros / 1000)
}

func (*SpanReader) buildServiceNameQuery(serviceName string) esquery.Query {
	return esquery.NewMatchQuery(serviceNameField, serviceName)
}

func (*SpanReader) buildOperationNameQuery(operationName string) esquery.Query {
	return esquery.NewMatchQuery(operationNameField, operationName)
}

func (s *SpanReader) buildTagQuery(k string, v string) esquery.Query {
	objectTagListLen := len(objectTagFieldList)
	queries := make([]esquery.Query, len(nestedTagFieldList)+objectTagListLen)
	kd := s.dotReplacer.ReplaceDot(k)
	for i := range objectTagFieldList {
		queries[i] = s.buildObjectQuery(objectTagFieldList[i], kd, v)
	}
	for i := range nestedTagFieldList {
		queries[i+objectTagListLen] = s.buildNestedQuery(nestedTagFieldList[i], k, v)
	}

	// but configuration can change over time
	return esquery.NewBoolQuery().Should(queries...)
}

func (*SpanReader) buildNestedQuery(field string, k string, v string) esquery.Query {
	keyField := fmt.Sprintf("%s.%s", field, tagKeyField)
	valueField := fmt.Sprintf("%s.%s", field, tagValueField)
	keyQuery := esquery.NewMatchQuery(keyField, k)
	valueQuery := esquery.NewRegexpQuery(valueField, v)
	tagBoolQuery := esquery.NewBoolQuery().Must(keyQuery, valueQuery)
	return esquery.NewNestedQuery(field, tagBoolQuery)
}

func (*SpanReader) buildObjectQuery(field string, k string, v string) esquery.Query {
	keyField := fmt.Sprintf("%s.%s", field, k)
	keyQuery := esquery.NewRegexpQuery(keyField, v)
	return esquery.NewBoolQuery().Must(keyQuery)
}

func (s *SpanReader) mergeAllNestedAndElevatedTagsOfSpan(span *dbmodel.Span) {
	processTags := s.mergeNestedAndElevatedTags(span.Process.Tags, span.Process.Tag)
	span.Process.Tags = processTags
	spanTags := s.mergeNestedAndElevatedTags(span.Tags, span.Tag)
	span.Tags = spanTags
}

func (s *SpanReader) mergeNestedAndElevatedTags(nestedTags []dbmodel.KeyValue, elevatedTags map[string]any) []dbmodel.KeyValue {
	mergedTags := make([]dbmodel.KeyValue, 0, len(nestedTags)+len(elevatedTags))
	mergedTags = append(mergedTags, nestedTags...)
	for k, v := range elevatedTags {
		kv := s.convertTagField(k, v)
		mergedTags = append(mergedTags, kv)
		delete(elevatedTags, k)
	}
	return mergedTags
}

func (s *SpanReader) convertTagField(k string, v any) dbmodel.KeyValue {
	dKey := s.dotReplacer.ReplaceDotReplacement(k)
	kv := dbmodel.KeyValue{
		Key:   dKey,
		Value: v,
	}
	switch val := v.(type) {
	case int64:
		kv.Type = dbmodel.Int64Type
	case float64:
		kv.Type = dbmodel.Float64Type
	case bool:
		kv.Type = dbmodel.BoolType
	case string:
		kv.Type = dbmodel.StringType
	// the binary is never returned, ES returns it as string with base64 encoding
	case []byte:
		kv.Type = dbmodel.BinaryType
	// in spans are decoded using json.UseNumber() to preserve the type
	// however note that float(1) will be parsed as int as ES does not store decimal point
	case json.Number:
		n, err := val.Int64()
		if err == nil {
			kv.Value = n
			kv.Type = dbmodel.Int64Type
		} else {
			f, err := val.Float64()
			if err != nil {
				return dbmodel.KeyValue{
					Key:   dKey,
					Value: fmt.Sprintf("invalid tag type in %+v: %s", v, err.Error()),
					Type:  dbmodel.StringType,
				}
			}
			kv.Value = f
			kv.Type = dbmodel.Float64Type
		}
	default:
		return dbmodel.KeyValue{
			Key:   dKey,
			Value: fmt.Sprintf("invalid tag type in %+v", v),
			Type:  dbmodel.StringType,
		}
	}
	return kv
}

func logErrorToSpan(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
