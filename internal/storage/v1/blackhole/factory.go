// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package blackhole

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/spanstore"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// interface comformance checks
var _ storage.Factory = (*Factory)(nil)

// Factory implements storage.Factory and creates blackhole storage components.
type Factory struct {
	metricsFactory metrics.Factory
	logger         *zap.Logger
	store          *Store
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	f.store = NewStore()
	logger.Info("Blackhole storage initialized")
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

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return f.store, nil
}
