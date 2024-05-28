// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package converter

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	storage_v1 "github.com/jaegertracing/jaeger/storage"
	dependencystore_v1 "github.com/jaegertracing/jaeger/storage/dependencystore"
	spanstore_v1 "github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type Factory struct {
	ss spanstore.Factory
}

func NewFactory(ss spanstore.Factory) storage_v1.Factory {
	return &Factory{
		ss: ss,
	}
}

func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	return f.ss.Initialize(context.Background())
}

func (f *Factory) CreateSpanReader() (spanstore_v1.Reader, error) {
	traceReader, err := f.ss.CreateTraceReader()
	if err != nil {
		return nil, err
	}

	return NewSpanReader(traceReader)
}

func (f *Factory) CreateSpanWriter() (spanstore_v1.Writer, error) {
	traceWriter, err := f.ss.CreateTraceWriter()
	if err != nil {
		return nil, err
	}

	return NewSpanWriter(traceWriter)
}

type UnimplementedDependencyStore struct{}

func (s *UnimplementedDependencyStore) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	panic("not implemented")
}

func (f *Factory) CreateDependencyReader() (dependencystore_v1.Reader, error) {
	return &UnimplementedDependencyStore{}, nil
}
