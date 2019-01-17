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

// Factory implements storage.Factory for Elasticsearch backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger

	primaryConfig config.ClientBuilder
	primaryClient es.Client
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		Options: NewOptions("es"), // TODO add "es-archive" once supported
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
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger

	primaryClient, err := f.primaryConfig.NewClient(logger, metricsFactory)
	if err != nil {
		return err
	}
	f.primaryClient = primaryClient
	// TODO init archive (cf. https://github.com/jaegertracing/jaeger/pull/604)
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	cfg := f.primaryConfig
	return esSpanStore.NewSpanReader(esSpanStore.SpanReaderParams{
		Client:            f.primaryClient,
		Logger:            f.logger,
		MetricsFactory:    f.metricsFactory,
		MaxSpanAge:        cfg.GetMaxSpanAge(),
		MaxNumSpans:       cfg.GetMaxNumSpans(),
		IndexPrefix:       cfg.GetIndexPrefix(),
		TagDotReplacement: cfg.GetTagDotReplacement(),
	}), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	cfg := f.primaryConfig
	var tags []string
	if cfg.GetTagsFilePath() != "" {
		var err error
		if tags, err = loadTagsFromFile(cfg.GetTagsFilePath()); err != nil {
			f.logger.Error("Could not open file with tags", zap.Error(err))
			return nil, err
		}
	}
	return esSpanStore.NewSpanWriter(esSpanStore.SpanWriterParams{Client: f.primaryClient,
		Logger:            f.logger,
		MetricsFactory:    f.metricsFactory,
		NumShards:         f.primaryConfig.GetNumShards(),
		NumReplicas:       f.primaryConfig.GetNumReplicas(),
		IndexPrefix:       f.primaryConfig.GetIndexPrefix(),
		AllTagsAsFields:   f.primaryConfig.GetAllTagsAsFields(),
		TagKeysAsFields:   tags,
		TagDotReplacement: f.primaryConfig.GetTagDotReplacement(),
	}), nil
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
