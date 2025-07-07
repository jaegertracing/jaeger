// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

type Factory struct {
	v1Factory *badger.Factory
}

func NewFactory(
	cfg badger.Config,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	v1Factory := badger.NewFactory()
	v1Factory.Config = &cfg
	err := v1Factory.Initialize(metricsFactory, logger)
	if err != nil {
		return nil, err
	}
	f := Factory{v1Factory: v1Factory}
	return &f, nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	v1Writer, _ := f.v1Factory.CreateSpanWriter() // error is always nil
	return v1adapter.NewTraceWriter(v1Writer), nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	v1Reader, _ := f.v1Factory.CreateSpanReader() // error is always nil
	return v1adapter.NewTraceReader(v1Reader), nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	v1Reader, _ := f.v1Factory.CreateDependencyReader() // error is always nil
	return v1adapter.NewDependencyReader(v1Reader), nil
}

func (f *Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	return f.v1Factory.CreateSamplingStore(maxBuckets)
}

func (f *Factory) CreateLock() (distributedlock.Lock, error) {
	return f.v1Factory.CreateLock()
}

func (f *Factory) Close() error {
	return f.v1Factory.Close()
}

func (f *Factory) Purge(ctx context.Context) error {
	return f.v1Factory.Purge(ctx)
}
