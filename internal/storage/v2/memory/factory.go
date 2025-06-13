// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/tracestoremetrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	_ tracestore.Factory           = (*Factory)(nil)
	_ storage.SamplingStoreFactory = (*Factory)(nil)
	_ storage.Purger               = (*Factory)(nil)
)

type Factory struct {
	store          *Store
	metricsFactory metrics.Factory
}

func NewFactory(cfg v1.Configuration, settings telemetry.Settings) (*Factory, error) {
	store, err := NewStore(cfg)
	if err != nil {
		return nil, err
	}
	return &Factory{
		store:          store,
		metricsFactory: settings.Metrics,
	}, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return tracestoremetrics.NewReaderDecorator(f.store, f.metricsFactory), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return f.store, nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	return f.store, nil
}

func (*Factory) CreateSamplingStore(buckets int) (samplingstore.Store, error) {
	return v1.NewSamplingStore(buckets), nil
}

func (*Factory) CreateLock() (distributedlock.Lock, error) {
	return &v1.Lock{}, nil
}

func (f *Factory) Purge(_ context.Context) error {
	return f.store.Purge()
}
