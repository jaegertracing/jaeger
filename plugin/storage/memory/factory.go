// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/safeexpvar"
	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
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
	options Options
	telset  telemetry.Setting
	store   *Store
}

// NewFactory creates a new Factory.
func NewFactory(telset telemetry.Setting) *Factory {
	return &Factory{
		telset: telset,
	}
}

// NewFactoryWithConfig is used from jaeger(v2).
func NewFactoryWithConfig(
	cfg Configuration,
	telset telemetry.Setting,
) *Factory {
	f := NewFactory(telset)
	f.configureFromOptions(Options{Configuration: cfg})
	_ = f.Initialize()
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
func (f *Factory) Initialize() error {
	f.store = WithConfiguration(f.options.Configuration)
	f.telset.Logger.Info("Memory storage initialized", zap.Any("configuration", f.store.defaultConfig))
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
