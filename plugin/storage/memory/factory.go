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

package memory

import (
	"context"
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/safeexpvar"
	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var ( // interface comformance checks
	_ storage.Factory              = (*Factory)(nil)
	_ storage.ArchiveFactory       = (*Factory)(nil)
	_ storage.SamplingStoreFactory = (*Factory)(nil)
	_ plugin.Configurable          = (*Factory)(nil)
	_ storage.Purger               = (*Factory)(nil)
)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	options        Options
	metricsFactory metrics.Factory
	logger         *zap.Logger
	store          *Store
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// NewFactoryWithConfig is used from jaeger(v2).
func NewFactoryWithConfig(
	cfg Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) *Factory {
	f := NewFactory()
	f.configureFromOptions(Options{Configuration: cfg})
	_ = f.Initialize(metricsFactory, logger)
	return f
}

// AddFlags implements plugin.Configurable
func (*Factory) AddFlags(flagSet *flag.FlagSet) {
	AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.options.InitFromViper(v)
}

// configureFromOptions initializes factory from the supplied options
func (f *Factory) configureFromOptions(opts Options) {
	f.options = opts
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	f.store = WithConfiguration(f.options.Configuration)
	logger.Info("Memory storage initialized", zap.Any("configuration", f.store.defaultConfig))
	f.publishOpts()

	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return f.store, nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return f.store, nil
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	return f.store, nil
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	return f.store, nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return f.store, nil
}

// CreateSamplingStore implements storage.SamplingStoreFactory
func (*Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	return NewSamplingStore(maxBuckets), nil
}

// CreateLock implements storage.SamplingStoreFactory
func (*Factory) CreateLock() (distributedlock.Lock, error) {
	return &lock{}, nil
}

func (f *Factory) publishOpts() {
	safeexpvar.SetInt("jaeger_storage_memory_max_traces", int64(f.options.Configuration.MaxTraces))
}

// Purge removes all data from the Factory's underlying Memory store.
// This function is intended for testing purposes only and should not be used in production environments.
func (f *Factory) Purge(ctx context.Context) error {
	f.store.purge(ctx)
	return nil
}
