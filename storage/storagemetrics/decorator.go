// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagemetrics

import (
	"io"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage/spanstore/spanstoremetrics"
)

type DecoratorFactory struct {
	delegate       storage.Factory
	metricsFactory metrics.Factory
}

func NewDecoratorFactory(f storage.Factory, mf metrics.Factory) *DecoratorFactory {
	return &DecoratorFactory{
		delegate:       f,
		metricsFactory: mf,
	}
}

func (df *DecoratorFactory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	return df.delegate.Initialize(metricsFactory, logger)
}

func (df *DecoratorFactory) CreateSpanReader() (spanstore.Reader, error) {
	sr, err := df.delegate.CreateSpanReader()
	if err != nil {
		return sr, err
	}
	sr = spanstoremetrics.NewReaderDecorator(sr, df.metricsFactory)
	return sr, nil
}

func (df *DecoratorFactory) CreateSpanWriter() (spanstore.Writer, error) {
	return df.delegate.CreateSpanWriter()
}

func (df *DecoratorFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	return df.delegate.CreateDependencyReader()
}

func (df *DecoratorFactory) Close() error {
	if closer, ok := df.delegate.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
