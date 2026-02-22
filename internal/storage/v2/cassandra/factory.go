// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	cspanstore "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/tracestoremetrics"
	ctracestore "github.com/jaegertracing/jaeger/internal/storage/v2/cassandra/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

type Factory struct {
	metricsFactory metrics.Factory
	logger         *zap.Logger
	v1Factory      *cassandra.Factory
	tracer         trace.TracerProvider
}

// NewFactory creates and initializes the factory
func NewFactory(opts cassandra.Options, telset telemetry.Settings) (*Factory, error) {
	f := &Factory{
		metricsFactory: telset.Metrics,
		logger:         telset.Logger,
		tracer:         telset.TracerProvider,
	}
	baseFactory, err := newFactoryWithConfig(opts, f.metricsFactory, f.logger, f.tracer)
	if err != nil {
		return nil, err
	}
	f.v1Factory = baseFactory
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	corereader, err := cspanstore.NewSpanReader(
		f.v1Factory.GetSession(),
		f.metricsFactory,
		f.logger,
		f.tracer.Tracer("cSpanStore.SpanReader"),
	)
	if err != nil {
		return nil, err
	}
	return tracestoremetrics.NewReaderDecorator(
		ctracestore.NewTraceReader(corereader),
		f.metricsFactory,
	), nil
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

func (f *Factory) CreateDependencyWriter() (depstore.Writer, error) {
	reader, err := f.v1Factory.CreateDependencyReader()
	if err != nil {
		return nil, err
	}
	// TODO: Update this when the v1 factory interface has CreateDependencyWriter
	if writer, ok := reader.(dependencystore.Writer); ok {
		return v1adapter.NewDependencyWriter(writer), nil
	}
	return nil, nil
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
	tracer trace.TracerProvider,
) (*cassandra.Factory, error) {
	f := cassandra.NewFactory()
	// use this to help with testing
	b := &withConfigBuilder{
		f:              f,
		opts:           &opts,
		metricsFactory: metricsFactory,
		logger:         logger,
		tracer:         tracer,
		initializer:    f.Initialize, // this can be mocked in tests
	}
	return b.build()
}

type withConfigBuilder struct {
	f              *cassandra.Factory
	opts           *cassandra.Options
	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider
	initializer    func(metricsFactory metrics.Factory, logger *zap.Logger, tracer trace.TracerProvider) error
}

func (b *withConfigBuilder) build() (*cassandra.Factory, error) {
	b.f.ConfigureFromOptions(b.opts)
	if err := b.opts.Configuration.Validate(); err != nil {
		return nil, err
	}
	err := b.initializer(b.metricsFactory, b.logger, b.tracer)
	if err != nil {
		return nil, err
	}
	return b.f, nil
}
