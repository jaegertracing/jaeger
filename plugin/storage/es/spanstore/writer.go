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
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/converter/json"
	jModel "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

const (
	spanType    = "span"
	serviceType = "service"

	defaultNumShards   = 5
	defaultNumReplicas = 1
)

type spanWriterMetrics struct {
	indexCreate *storageMetrics.WriteMetrics
	spans       *storageMetrics.WriteMetrics
}

type serviceWriter func(string, *jModel.Span) error

// SpanWriter is a wrapper around elastic.Client
type SpanWriter struct {
	ctx           context.Context
	client        es.Client
	logger        *zap.Logger
	writerMetrics spanWriterMetrics // TODO: build functions to wrap around each Do fn
	indexCache    cache.Cache
	serviceWriter serviceWriter
	numShards     int64
	numReplicas   int64
}

// Service is the JSON struct for service:operation documents in ElasticSearch
type Service struct {
	ServiceName   string `json:"serviceName"`
	OperationName string `json:"operationName"`
}

// Span adds a StartTimeMillis field to the standard JSON span.
// ElasticSearch does not support a UNIX Epoch timestamp in microseconds,
// so Jaeger maps StartTime to a 'long' type. This extra StartTimeMillis field
// works around this issue, enabling timerange queries.
type Span struct {
	*jModel.Span
	StartTimeMillis uint64 `json:"startTimeMillis"`
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(
	client es.Client,
	logger *zap.Logger,
	metricsFactory metrics.Factory,
	numShards int64,
	numReplicas int64,
) *SpanWriter {
	ctx := context.Background()
	if numShards == 0 {
		numShards = defaultNumShards
	}
	if numReplicas == 0 {
		numReplicas = defaultNumReplicas
	}
	// TODO: Configurable TTL
	serviceOperationStorage := NewServiceOperationStorage(ctx, client, metricsFactory, logger, time.Hour*12)
	return &SpanWriter{
		ctx:    ctx,
		client: client,
		logger: logger,
		writerMetrics: spanWriterMetrics{
			indexCreate: storageMetrics.NewWriteMetrics(metricsFactory, "IndexCreate"),
			spans:       storageMetrics.NewWriteMetrics(metricsFactory, "Spans"),
		},
		serviceWriter: serviceOperationStorage.Write,
		indexCache: cache.NewLRUWithOptions(
			5,
			&cache.Options{
				TTL: 48 * time.Hour,
			},
		),
		numShards:   numShards,
		numReplicas: numReplicas,
	}
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(span *model.Span) error {
	spanIndexName, serviceIndexName := indexNames(span)
	// Convert model.Span into json.Span
	jsonSpan := json.FromDomainEmbedProcess(span)

	if err := s.createIndex(serviceIndexName, serviceMapping, jsonSpan); err != nil {
		return err
	}
	if err := s.writeService(serviceIndexName, jsonSpan); err != nil {
		return err
	}
	if err := s.createIndex(spanIndexName, spanMapping, jsonSpan); err != nil {
		return err
	}
	return s.writeSpan(spanIndexName, jsonSpan)
}

func indexNames(span *model.Span) (string, string) {
	spanDate := span.StartTime.Format("2006-01-02")
	return spanIndexPrefix + spanDate, serviceIndexPrefix + spanDate
}

func (s *SpanWriter) createIndex(indexName string, mapping string, jsonSpan *jModel.Span) error {
	if !keyInCache(indexName, s.indexCache) {
		start := time.Now()
		exists, _ := s.client.IndexExists(indexName).Do(s.ctx) // don't need to check the error because the exists variable will be false anyway if there is an error
		if !exists {
			// if there are multiple collectors writing to the same elasticsearch host, if the collectors pass
			// the exists check above and try to create the same index all at once, this might fail and
			// drop a couple spans (~1 per collector). Creating indices ahead of time alleviates this issue.
			_, err := s.client.CreateIndex(indexName).Body(s.fixMapping(mapping)).Do(s.ctx)
			s.writerMetrics.indexCreate.Emit(err, time.Since(start))
			if err != nil {
				return s.logError(jsonSpan, err, "Failed to create index", s.logger)
			}
		}
		writeCache(indexName, s.indexCache)
	}
	return nil
}

func keyInCache(key string, c cache.Cache) bool {
	return c.Get(key) != nil
}

func writeCache(key string, c cache.Cache) {
	c.Put(key, key)
}

func (s *SpanWriter) fixMapping(mapping string) string {
	mapping = strings.Replace(mapping, "${__NUMBER_OF_SHARDS__}", strconv.FormatInt(s.numShards, 10), 1)
	mapping = strings.Replace(mapping, "${__NUMBER_OF_REPLICAS__}", strconv.FormatInt(s.numReplicas, 10), 1)
	return mapping
}

func (s *SpanWriter) writeService(indexName string, jsonSpan *jModel.Span) error {
	return s.serviceWriter(indexName, jsonSpan)
}

func (s *SpanWriter) writeSpan(indexName string, jsonSpan *jModel.Span) error {
	start := time.Now()
	elasticSpan := Span{Span: jsonSpan, StartTimeMillis: jsonSpan.StartTime / 1000} // Microseconds to milliseconds
	_, err := s.client.Index().Index(indexName).Type(spanType).BodyJson(&elasticSpan).Do(s.ctx)
	s.writerMetrics.spans.Emit(err, time.Since(start))
	if err != nil {
		return s.logError(jsonSpan, err, "Failed to insert span", s.logger)
	}
	return nil
}

func (s *SpanWriter) logError(span *jModel.Span, err error, msg string, logger *zap.Logger) error {
	logger.
		With(zap.String("trace_id", string(span.TraceID))).
		With(zap.String("span_id", string(span.SpanID))).
		With(zap.Error(err)).
		Error(msg)
	return errors.Wrap(err, msg)
}
