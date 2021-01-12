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

package elasticsearchexporter

import (
	"context"
	"fmt"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter/esmodeltranslator"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/reader/es/esdependencyreader"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/reader/es/esspanreader"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const archiveNamespace = "es-archive"

// StorageFactory implements storage.Factory and storage.ArchiveFactory
type StorageFactory struct {
	options *es.Options
	name    string
	logger  *zap.Logger
}

var _ storage.Factory = (*StorageFactory)(nil)
var _ storage.ArchiveFactory = (*StorageFactory)(nil)

// NewStorageFactory creates StorageFactory
func NewStorageFactory(opts *es.Options, logger *zap.Logger, name string) *StorageFactory {
	return &StorageFactory{
		options: opts,
		logger:  logger,
		name:    name,
	}
}

// Initialize initializes StorageFactory
func (s *StorageFactory) Initialize(_ metrics.Factory, logger *zap.Logger) error {
	s.logger = logger
	return nil
}

// CreateSpanWriter creates spanstore.Writer
func (s *StorageFactory) CreateSpanWriter() (spanstore.Writer, error) {
	cfg := s.options.GetPrimary()
	writer, err := newEsSpanWriter(*cfg, s.logger, false, s.name)
	if err != nil {
		return nil, err
	}
	fields, err := cfg.TagKeysAsFields()
	if err != nil {
		return nil, err
	}
	return &singleSpanWriter{
		converter: dbmodel.NewFromDomain(cfg.GetAllTagsAsFields(), fields, cfg.GetTagDotReplacement()),
		writer:    writer,
	}, nil
}

// CreateSpanReader creates spanstore.Reader
func (s *StorageFactory) CreateSpanReader() (spanstore.Reader, error) {
	cfg := s.options.GetPrimary()
	client, err := esclient.NewElasticsearchClient(*cfg, s.logger)
	if err != nil {
		return nil, err
	}
	return esspanreader.NewEsSpanReader(client, s.logger, esspanreader.Config{
		Archive:             false,
		UseReadWriteAliases: cfg.GetUseReadWriteAliases(),
		IndexPrefix:         cfg.GetIndexPrefix(),
		IndexDateLayout:     cfg.GetIndexDateLayout(),
		MaxSpanAge:          cfg.GetMaxSpanAge(),
		MaxDocCount:         cfg.GetMaxDocCount(),
		TagDotReplacement:   cfg.GetTagDotReplacement(),
	}), nil
}

// CreateDependencyReader creates dependencystore.Reader
func (s *StorageFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	cfg := s.options.GetPrimary()
	client, err := esclient.NewElasticsearchClient(*cfg, s.logger)
	if err != nil {
		return nil, err
	}
	return esdependencyreader.NewDependencyStore(client, s.logger, cfg.GetIndexPrefix(), cfg.GetIndexDateLayout(), cfg.GetMaxDocCount()), nil
}

// CreateArchiveSpanReader creates archive spanstore.Reader
func (s *StorageFactory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	cfg := s.options.Get(archiveNamespace)
	client, err := esclient.NewElasticsearchClient(*cfg, s.logger)
	if err != nil {
		return nil, err
	}
	return esspanreader.NewEsSpanReader(client, s.logger, esspanreader.Config{
		Archive:             true,
		UseReadWriteAliases: cfg.GetUseReadWriteAliases(),
		IndexPrefix:         cfg.GetIndexPrefix(),
		IndexDateLayout:     cfg.GetIndexDateLayout(),
		MaxSpanAge:          cfg.GetMaxSpanAge(),
		MaxDocCount:         cfg.GetMaxDocCount(),
		TagDotReplacement:   cfg.GetTagDotReplacement(),
	}), nil
}

// CreateArchiveSpanWriter creates archive spanstore.Writer
func (s *StorageFactory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	cfg := s.options.Get(archiveNamespace)
	writer, err := newEsSpanWriter(*cfg, s.logger, true, fmt.Sprintf("%s/%s", s.name, archiveNamespace))
	if err != nil {
		return nil, err
	}
	fields, err := cfg.TagKeysAsFields()
	if err != nil {
		return nil, err
	}
	return &singleSpanWriter{
		converter: dbmodel.NewFromDomain(cfg.GetAllTagsAsFields(), fields, cfg.GetTagDotReplacement()),
		writer:    writer,
	}, nil
}

type singleSpanWriter struct {
	writer    batchSpanWriter
	converter dbmodel.FromDomain
}

type batchSpanWriter interface {
	writeSpans(context.Context, []esmodeltranslator.ConvertedData) (int, error)
}

var _ spanstore.Writer = (*singleSpanWriter)(nil)

func (s singleSpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	dbSpan := s.converter.FromDomainEmbedProcess(span)
	_, err := s.writer.writeSpans(ctx, []esmodeltranslator.ConvertedData{{DBSpan: dbSpan}})
	return err
}
