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
	"context"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

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
	client           es.Client
	logger           *zap.Logger
	writerMetrics    spanWriterMetrics // TODO: build functions to wrap around each Do fn
	indexCache       cache.Cache
	serviceWriter    serviceWriter
	spanConverter    dbmodel.FromDomain
	spanServiceIndex spanAndServiceIndexFn
}

// SpanWriterParams holds constructor parameters for NewSpanWriter
type SpanWriterParams struct {
	Client              es.Client
	Logger              *zap.Logger
	MetricsFactory      metrics.Factory
	IndexPrefix         string
	IndexDateLayout     string
	AllTagsAsFields     bool
	TagKeysAsFields     []string
	TagDotReplacement   string
	Archive             bool
	UseReadWriteAliases bool
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(p SpanWriterParams) *SpanWriter {
	// TODO: Configurable TTL
	serviceOperationStorage := NewServiceOperationStorage(p.Client, p.Logger, time.Hour*12)
	return &SpanWriter{
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
		spanConverter:    dbmodel.NewFromDomain(p.AllTagsAsFields, p.TagKeysAsFields, p.TagDotReplacement),
		spanServiceIndex: getSpanAndServiceIndexFn(p.Archive, p.UseReadWriteAliases, p.IndexPrefix, p.IndexDateLayout),
	}
}

// CreateTemplates creates index templates.
func (s *SpanWriter) CreateTemplates(spanTemplate, serviceTemplate string) error {
	_, err := s.client.CreateTemplate("jaeger-span").Body(spanTemplate).Do(context.Background())
	if err != nil {
		return err
	}
	_, err = s.client.CreateTemplate("jaeger-service").Body(serviceTemplate).Do(context.Background())
	if err != nil {
		return err
	}
	return nil
}

// spanAndServiceIndexFn returns names of span and service indices
type spanAndServiceIndexFn func(spanTime time.Time) (string, string)

func getSpanAndServiceIndexFn(archive, useReadWriteAliases bool, prefix, dateLayout string) spanAndServiceIndexFn {
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
		return indexWithDate(spanIndexPrefix, dateLayout, date), indexWithDate(serviceIndexPrefix, dateLayout, date)
	}
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(_ context.Context, span *model.Span) error {
	spanIndexName, serviceIndexName := s.spanServiceIndex(span.StartTime)
	jsonSpan := s.spanConverter.FromDomainEmbedProcess(span)
	if serviceIndexName != "" {
		s.writeService(serviceIndexName, jsonSpan)
	}
	s.writeSpan(spanIndexName, jsonSpan)
	return nil
}

// Close closes SpanWriter
func (s *SpanWriter) Close() error {
	return s.client.Close()
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
