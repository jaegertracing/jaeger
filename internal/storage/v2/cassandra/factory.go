// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"context"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

type Factory struct {
	v1Factory *cassandra.Factory
}

// NewFactory creates and initializes the factory
func NewFactory(opts cassandra.Options, metricsFactory metrics.Factory, logger *zap.Logger) (*Factory, error) {
	factory, err := newFactoryWithConfig(opts, metricsFactory, logger)
	if err != nil {
		return nil, err
	}
	return &Factory{v1Factory: factory}, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	reader, err := f.v1Factory.CreateSpanReader()
	if err != nil {
		return nil, err
	}
	return v1adapter.NewTraceReader(reader), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	writer, err := f.v1Factory.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	return v1adapter.NewTraceWriter(writer), nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	reader, err := f.v1Factory.CreateDependencyReader()
	if err != nil {
		return nil, err
	}
	return v1adapter.NewDependencyReader(reader), nil
}

func (f *Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	return f.v1Factory.CreateSamplingStore(maxBuckets)
}

func (f *Factory) Close() error {
	return f.v1Factory.Close()
}

func (f *Factory) Purge(ctx context.Context) error {
	return f.v1Factory.Purge(ctx)
}

func (f *Factory) CreateLock() (distributedlock.Lock, error) {
	return f.v1Factory.CreateLock()
}

// newFactoryWithConfig initializes factory with Config.
func newFactoryWithConfig(
	opts cassandra.Options,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*cassandra.Factory, error) {
	f := cassandra.NewFactory()
	// use this to help with testing
	b := &withConfigBuilder{
		f:              f,
		opts:           &opts,
		metricsFactory: metricsFactory,
		logger:         logger,
		initializer:    f.Initialize, // this can be mocked in tests
	}
	return b.build()
}

type withConfigBuilder struct {
	f              *cassandra.Factory
	opts           *cassandra.Options
	metricsFactory metrics.Factory
	logger         *zap.Logger
	initializer    func(metricsFactory metrics.Factory, logger *zap.Logger) error
}

func (b *withConfigBuilder) build() (*cassandra.Factory, error) {
	b.f.ConfigureFromOptions(b.opts)
	if err := b.opts.NamespaceConfig.Validate(); err != nil {
		return nil, err
	}
	err := b.initializer(b.metricsFactory, b.logger)
	if err != nil {
		return nil, err
	}
	return b.f, nil
}
