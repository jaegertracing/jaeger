// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

// NewFactory creates and initializes the factory
func NewFactory(opts cassandra.Options, metricsFactory metrics.Factory, logger *zap.Logger) (tracestore.Factory, error) {
	return newFactory(func() (v1Factory *cassandra.Factory, err error) {
		return cassandra.NewFactoryWithConfig(opts, metricsFactory, logger)
	})
}

func newFactory(factoryBuilder func() (v1Factory *cassandra.Factory, err error)) (tracestore.Factory, error) {
	v1Factory, err := factoryBuilder()
	if err != nil {
		return nil, err
	}
	return v1adapter.NewFactory(v1Factory), nil
}
