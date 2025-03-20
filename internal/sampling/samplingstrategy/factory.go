// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstrategy

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
)

// Factory defines an interface for a factory that can create implementations of different sampling strategy components.
// Implementations are also encouraged to implement storage.Configurable interface.
//
// # See also
//
// storage.Configurable
type Factory interface {
	// Initialize performs internal initialization of the factory.
	Initialize(metricsFactory api.Factory, ssFactory storage.SamplingStoreFactory, logger *zap.Logger) error

	// CreateStrategyProvider initializes and returns Provider and optionallty Aggregator.
	CreateStrategyProvider() (Provider, Aggregator, error)

	// Close closes the factory
	Close() error
}
