// Copyright (c) 2020 The Jaeger Authors.
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

package esspanreader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esutil"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	// by default UI fetches 20 results
	defaultNumTraces = 20

	spanIndexBaseName    = "jaeger-span"
	serviceIndexBaseName = "jaeger-service"

	operationNameField      = "operationName"
	serviceNameField        = "serviceName"
	traceIDField            = "traceID"
	startTimeField          = "startTime"
	durationField           = "duration"
	objectTagsField         = "tag"
	objectProcessTagsField  = "process.tag"
	nestedProcessTagsField  = "process.tags"
	nestedTagsField         = "tags"
	nestedLogFieldsField    = "logs.fields"
	processServiceNameField = "process.serviceName"
	tagKeyField             = "key"
	tagValueField           = "value"
)

// Reader defines Elasticsearch reader.
type Reader struct {
	logger           *zap.Logger
	client           esclient.ElasticsearchClient
	converter        dbmodel.ToDomain
	serviceIndexName esutil.IndexNameProvider
	spanIndexName    esutil.IndexNameProvider
	maxSpanAge       time.Duration
	maxDocCount      int
	archive          bool
}

var _ spanstore.Reader = (*Reader)(nil)

// Config defines configuration for span reader.
type Config struct {
	Archive             bool
	UseReadWriteAliases bool
	IndexPrefix         string
	IndexDateLayout     string
	MaxSpanAge          time.Duration
	MaxDocCount         int
	TagDotReplacement   string
}

// NewEsSpanReader creates Elasticseach span reader.
func NewEsSpanReader(client esclient.ElasticsearchClient, logger *zap.Logger, config Config) *Reader {
	alias := esutil.AliasNone
	if config.UseReadWriteAliases {
		alias = esutil.AliasRead
	}
	return &Reader{
		client:           client,
		logger:           logger,
		archive:          config.Archive,
		maxSpanAge:       config.MaxSpanAge,
		maxDocCount:      config.MaxDocCount,
		converter:        dbmodel.NewToDomain(config.TagDotReplacement),
		spanIndexName:    esutil.NewIndexNameProvider(spanIndexBaseName, config.IndexPrefix, config.IndexDateLayout, alias, config.Archive),
		serviceIndexName: esutil.NewIndexNameProvider(serviceIndexBaseName, config.IndexPrefix, config.IndexDateLayout, alias, config.Archive),
	}
}

// GetTrace implements spanstore.Reader
func (r *Reader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	currentTime := time.Now()
	traces, err := r.traceIDsMultiSearch(ctx, []model.TraceID{traceID}, currentTime.Add(-r.maxSpanAge), currentTime)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return traces[0], nil
}

// FindTraces implements spanstore.Reader
func (r *Reader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	uniqueTraceIDs, err := r.FindTraceIDs(ctx, query)
	if err != nil {
		return nil, err
	}
	return r.traceIDsMultiSearch(ctx, uniqueTraceIDs, query.StartTimeMin, query.StartTimeMax)
}

// FindTraceIDs implements spanstore.Reader
func (r *Reader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	if err := validateQuery(query); err != nil {
		return nil, err
	}
	if query.NumTraces == 0 {
		query.NumTraces = defaultNumTraces
	}
	esTraceIDs, err := r.findTraceIDs(ctx, query)
	if err != nil {
		return nil, err
	}
	return convertTraceIDsStringsToModel(esTraceIDs)
}

func convertTraceIDsStringsToModel(traceIDs []string) ([]model.TraceID, error) {
	traceIDsModels := make([]model.TraceID, 0, len(traceIDs))
	for _, ID := range traceIDs {
		traceID, err := model.TraceIDFromString(ID)
		if err != nil {
			return nil, fmt.Errorf("making traceID from string '%s' failed: %w", ID, err)
		}
		traceIDsModels = append(traceIDsModels, traceID)
	}
	return traceIDsModels, nil
}

func (r *Reader) findTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]string, error) {
	searchBody := findTraceIDsSearchBody(r.converter, query)
	indices := r.spanIndexName.IndexNameRange(query.StartTimeMin, query.StartTimeMax)
	response, err := r.client.Search(ctx, searchBody, 0, indices...)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("%s", response.Error)
	}

	var traceIDs []string
	for _, k := range response.Aggs[traceIDField].Buckets {
		traceIDs = append(traceIDs, k.Key)
	}
	return traceIDs, nil
}

// GetServices implements spanstore.Reader
func (r *Reader) GetServices(ctx context.Context) ([]string, error) {
	searchBody := getServicesSearchBody(r.maxDocCount)
	currentTime := time.Now()
	indices := r.serviceIndexName.IndexNameRange(currentTime.Add(-r.maxSpanAge), currentTime)
	response, err := r.client.Search(ctx, searchBody, 0, indices...)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("%s", response.Error)
	}

	var serviceNames []string
	for _, k := range response.Aggs[serviceNameField].Buckets {
		serviceNames = append(serviceNames, k.Key)
	}
	return serviceNames, nil
}

// GetOperations implements spanstore.Reader
func (r *Reader) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	searchBody := getOperationsSearchBody(query.ServiceName, r.maxDocCount)
	currentTime := time.Now()
	indices := r.serviceIndexName.IndexNameRange(currentTime.Add(-r.maxSpanAge), currentTime)
	response, err := r.client.Search(ctx, searchBody, 0, indices...)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("%s", response.Error)
	}

	var operations []spanstore.Operation
	for _, k := range response.Aggs[operationNameField].Buckets {
		operations = append(operations, spanstore.Operation{
			Name: k.Key,
		})
	}
	return operations, nil
}

// traceIDsMultiSearch invokes ES multi search API to search for traces with given traceIDs
func (r *Reader) traceIDsMultiSearch(ctx context.Context, traceIDs []model.TraceID, startTime, endTime time.Time) ([]*model.Trace, error) {
	if len(traceIDs) == 0 {
		return []*model.Trace{}, nil
	}

	indices := r.spanIndexName.IndexNameRange(startTime.Add(-time.Hour), endTime.Add(time.Hour))
	nextTime := model.TimeAsEpochMicroseconds(startTime.Add(-time.Hour))
	tracesMap := make(map[model.TraceID]*model.Trace)
	searchAfterTime := make(map[model.TraceID]uint64)
	totalFetchedSpans := make(map[model.TraceID]int)

	// The loop creates request for each traceID
	// If the response has more total hits than fetched spans
	// then second query for a trace is initiated with later timestamp.
	for {
		if len(traceIDs) == 0 {
			break
		}
		searchRequests, nt := r.multiSearchRequests(indices, traceIDs, searchAfterTime, nextTime)
		nextTime = nt
		traceIDs = nil // set traceIDs to empty
		response, err := r.client.MultiSearch(ctx, searchRequests)
		if err != nil {
			return nil, err
		}

		for _, resp := range response.Responses {
			if resp.Error != nil {
				return nil, fmt.Errorf("%s", resp.Error)
			}
			if resp.Hits.Total == 0 {
				continue
			}
			spans, err := r.getSpans(resp)
			if err != nil {
				return nil, err
			}

			lastSpan := spans[len(spans)-1]
			if trace, ok := tracesMap[lastSpan.TraceID]; ok {
				trace.Spans = append(trace.Spans, spans...)
			} else {
				tracesMap[lastSpan.TraceID] = &model.Trace{Spans: spans}
			}

			totalFetchedSpans[lastSpan.TraceID] = totalFetchedSpans[lastSpan.TraceID] + len(resp.Hits.Hits)
			if totalFetchedSpans[lastSpan.TraceID] < resp.Hits.Total {
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

func (r *Reader) getSpans(response esclient.SearchResponse) ([]*model.Span, error) {
	spans := make([]*model.Span, len(response.Hits.Hits))
	for i, hit := range response.Hits.Hits {
		var dbSpan *dbmodel.Span
		d := json.NewDecoder(bytes.NewReader(*hit.Source))
		d.UseNumber()
		if err := d.Decode(&dbSpan); err != nil {
			return nil, err
		}
		span, err := r.converter.SpanToDomain(dbSpan)
		if err != nil {
			return nil, err
		}
		spans[i] = span
	}
	return spans, nil
}

func (r *Reader) multiSearchRequests(indices []string, traceIDs []model.TraceID, searchAfterTime map[model.TraceID]uint64, nextTime uint64) ([]esclient.SearchBody, uint64) {
	queries := make([]esclient.SearchBody, len(traceIDs))
	for i, traceID := range traceIDs {
		if v, ok := searchAfterTime[traceID]; ok {
			nextTime = v
		}
		s := esclient.SearchBody{
			Indices:        indices,
			Query:          traceIDQuery(traceID),
			Size:           r.maxDocCount,
			TerminateAfter: r.maxDocCount,
		}
		if !r.archive {
			s.SearchAfter = []interface{}{nextTime}
			s.Sort = []map[string]esclient.Order{{startTimeField: esclient.AscOrder}}
		}
		queries[i] = s
	}
	return queries, nextTime
}
