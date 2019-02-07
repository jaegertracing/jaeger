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

package es

import (
	"bufio"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	esDepStore "github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore"
	esSpanStore "github.com/jaegertracing/jaeger/plugin/storage/es/spanstore"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	primaryNamespace = "es"
	archiveNamespace = "es-archive"
)

// Factory implements storage.Factory for Elasticsearch backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger

	primaryConfig config.ClientBuilder
	primaryClient es.Client
	archiveConfig config.ClientBuilder
	archiveClient es.Client
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		Options: NewOptions(primaryNamespace, archiveNamespace),
	}
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper) {
	f.Options.InitFromViper(v)
	f.primaryConfig = f.Options.GetPrimary()
	f.archiveConfig = f.Options.Get(archiveNamespace)
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger

	primaryClient, err := f.primaryConfig.NewClient(logger, metricsFactory)
	if err != nil {
		f.logger.Error("failed to create primary Elasticsearch client", zap.Error(err))
		return err
	}
	f.primaryClient = primaryClient
	archiveClient, err := f.archiveConfig.NewClient(logger, metricsFactory)
	if err != nil {
		f.logger.Error("failed to create archive Elasticsearch client", zap.Error(err))
		return err
	}
	f.archiveClient = archiveClient
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return createSpanReader(f.metricsFactory, f.logger, f.primaryClient, f.primaryConfig, false)
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return createSpanWriter(f.metricsFactory, f.logger, f.primaryClient, f.primaryConfig, false)
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return esDepStore.NewDependencyStore(f.primaryClient, f.logger, f.primaryConfig.GetIndexPrefix()), nil
}

func loadTagsFromFile(filePath string) ([]string, error) {
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var tags []string
	for scanner.Scan() {
		line := scanner.Text()
		if tag := strings.TrimSpace(line); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	return createSpanReader(f.metricsFactory, f.logger, f.archiveClient, f.Options.Get(archiveNamespace), true)
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	return createSpanWriter(f.metricsFactory, f.logger, f.archiveClient, f.Options.Get(archiveNamespace), true)
}

func createSpanReader(
	mFactory metrics.Factory,
	logger *zap.Logger,
	client es.Client,
	cfg config.ClientBuilder,
	archive bool,
) (spanstore.Reader, error) {
	return esSpanStore.NewSpanReader(esSpanStore.SpanReaderParams{
		Client:              client,
		Logger:              logger,
		MetricsFactory:      mFactory,
		MaxNumSpans:         cfg.GetMaxNumSpans(),
		MaxSpanAge:          cfg.GetMaxSpanAge(),
		IndexPrefix:         cfg.GetIndexPrefix(),
		TagDotReplacement:   cfg.GetTagDotReplacement(),
		UseReadWriteAliases: cfg.GetUseReadWriteAliases(),
		Archive:             archive,
	}), nil
}

func createSpanWriter(
	mFactory metrics.Factory,
	logger *zap.Logger,
	client es.Client,
	cfg config.ClientBuilder,
	archive bool,
) (spanstore.Writer, error) {
	var tags []string
	if cfg.GetTagsFilePath() != "" {
		var err error
		if tags, err = loadTagsFromFile(cfg.GetTagsFilePath()); err != nil {
			logger.Error("Could not open file with tags", zap.Error(err))
			return nil, err
		}
	}
	return esSpanStore.NewSpanWriter(esSpanStore.SpanWriterParams{
		Client:              client,
		Logger:              logger,
		MetricsFactory:      mFactory,
		NumShards:           cfg.GetNumShards(),
		NumReplicas:         cfg.GetNumReplicas(),
		IndexPrefix:         cfg.GetIndexPrefix(),
		AllTagsAsFields:     cfg.GetAllTagsAsFields(),
		TagKeysAsFields:     tags,
		TagDotReplacement:   cfg.GetTagDotReplacement(),
		Archive:             archive,
		UseReadWriteAliases: cfg.GetUseReadWriteAliases(),
	}), nil
}
