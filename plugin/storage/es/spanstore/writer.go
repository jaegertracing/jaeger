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
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/olivere/elastic.v5"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

const (
	spanType    = "span"
	serviceType = "service"
)

type spanWriterMetrics struct {
	indexCreate *storageMetrics.WriteMetrics
}

type serviceWriter func(string, *dbmodel.Span)

// SpanWriter is a wrapper around elastic.Client
type SpanWriter struct {
	ctx              context.Context
	client           es.Client
	logger           *zap.Logger
	writerMetrics    spanWriterMetrics // TODO: build functions to wrap around each Do fn
	indexCache       cache.Cache
	indexMutex       sync.Mutex
	serviceWriter    serviceWriter
	numShards        int64
	numReplicas      int64
	spanConverter    dbmodel.FromDomain
	spanServiceIndex spanAndServiceIndexFn
	spanMapping      string
	serviceMapping   string
}

// SpanWriterParams holds constructor parameters for NewSpanWriter
type SpanWriterParams struct {
	Client              es.Client
	Logger              *zap.Logger
	MetricsFactory      metrics.Factory
	IndexPrefix         string
	AllTagsAsFields     bool
	TagKeysAsFields     []string
	TagDotReplacement   string
	Archive             bool
	UseReadWriteAliases bool
	SpanMapping         string
	ServiceMapping      string
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(p SpanWriterParams) *SpanWriter {
	ctx := context.Background()

	// TODO: Configurable TTL
	serviceOperationStorage := NewServiceOperationStorage(ctx, p.Client, p.Logger, time.Hour*12)
	return &SpanWriter{
		ctx:    ctx,
		client: p.Client,
		logger: p.Logger,
		writerMetrics: spanWriterMetrics{
			indexCreate: storageMetrics.NewWriteMetrics(p.MetricsFactory, "index_create"),
		},
		serviceWriter: serviceOperationStorage.Write,
		indexCache: cache.NewLRUWithOptions(
			5,
			&cache.Options{
				TTL: 48 * time.Hour,
			},
		),
		spanMapping:      p.SpanMapping,
		serviceMapping:   p.ServiceMapping,
		spanConverter:    dbmodel.NewFromDomain(p.AllTagsAsFields, p.TagKeysAsFields, p.TagDotReplacement),
		spanServiceIndex: getSpanAndServiceIndexFn(p.Archive, p.UseReadWriteAliases, p.IndexPrefix),
	}
}

// spanAndServiceIndexFn returns names of span and service indices
type spanAndServiceIndexFn func(spanTime time.Time) (string, string)

func getSpanAndServiceIndexFn(archive, useReadWriteAliases bool, prefix string) spanAndServiceIndexFn {
	if prefix != "" {
		prefix += indexPrefixSeparator
	}
	spanIndexPrefix := prefix + spanIndex
	serviceIndexPrefix := prefix + serviceIndex
	if archive {
		return func(date time.Time) (string, string) {
			if useReadWriteAliases {
				return archiveIndex(spanIndexPrefix, archiveWriteIndexSuffix), ""
			}
			return archiveIndex(spanIndexPrefix, archiveIndexSuffix), ""
		}
	}

	if useReadWriteAliases {
		return func(spanTime time.Time) (string, string) {
			return spanIndexPrefix + "write", serviceIndexPrefix + "write"
		}
	}
	return func(date time.Time) (string, string) {
		return indexWithDate(spanIndexPrefix, date), indexWithDate(serviceIndexPrefix, date)
	}
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(span *model.Span) error {
	spanIndexName, serviceIndexName := s.spanServiceIndex(span.StartTime)
	jsonSpan := s.spanConverter.FromDomainEmbedProcess(span)
	if serviceIndexName != "" {
		if err := s.createIndex(serviceIndexName, s.serviceMapping, jsonSpan); err != nil {
			return err
		}
		s.writeService(serviceIndexName, jsonSpan)
	}
	if err := s.createIndex(spanIndexName, s.spanMapping, jsonSpan); err != nil {
		return err
	}
	s.writeSpan(spanIndexName, jsonSpan)
	return nil
}

// Close closes SpanWriter
func (s *SpanWriter) Close() error {
	return s.client.Close()
}

func (s *SpanWriter) createIndex(indexName string, mapping string, jsonSpan *dbmodel.Span) error {
	if !keyInCache(indexName, s.indexCache) {
		s.indexMutex.Lock()
		defer s.indexMutex.Unlock()

		// re-check if index exists in case other goroutine did the job under lock for us
		if keyInCache(indexName, s.indexCache) {
			return nil
		}

		start := time.Now()
		exists, _ := s.client.IndexExists(indexName).Do(s.ctx) // don't need to check the error because the exists variable will be false anyway if there is an error
		if !exists {
			// if there are multiple collectors writing to the same elasticsearch host a race condition can occur - create the index multiple times
			// we check for the error type to minimize errors
			_, err := s.client.CreateIndex(indexName).Body(mapping).Do(s.ctx)
			s.writerMetrics.indexCreate.Emit(err, time.Since(start))
			if err != nil {
				eErr, ok := err.(*elastic.Error)
				if !ok || eErr.Details != nil &&
					// ES 5.x
					(eErr.Details.Type != "index_already_exists_exception" &&
						// ES 6.x
						eErr.Details.Type != "resource_already_exists_exception") {
					return s.logError(jsonSpan, err, "Failed to create index", s.logger)
				}
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

func (s *SpanWriter) writeService(indexName string, jsonSpan *dbmodel.Span) {
	s.serviceWriter(indexName, jsonSpan)
}

func (s *SpanWriter) writeSpan(indexName string, jsonSpan *dbmodel.Span) {
	s.client.Index().Index(indexName).Type(spanType).BodyJson(&jsonSpan).Add()
}

func (s *SpanWriter) logError(span *dbmodel.Span, err error, msg string, logger *zap.Logger) error {
	logger.
		With(zap.String("trace_id", string(span.TraceID))).
		With(zap.String("span_id", string(span.SpanID))).
		With(zap.Error(err)).
		Error(msg)
	return errors.Wrap(err, msg)
}
