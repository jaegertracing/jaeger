// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// BaseFactory is the same as Factory, but without the Initialize method.
// It was a design mistake originally to add Initialize to the Factory interface.
type BaseFactory interface {
	// CreateSpanReader creates a spanstore.Reader.
	CreateSpanReader() (spanstore.Reader, error)

	// CreateSpanWriter creates a spanstore.Writer.
	CreateSpanWriter() (spanstore.Writer, error)

	// CreateDependencyReader creates a dependencystore.Reader.
	CreateDependencyReader() (dependencystore.Reader, error)
}

// Factory defines an interface for a factory that can create implementations of different storage components.
// Implementations are also encouraged to implement storage.Configurable interface.
//
// # See also
//
// storage.Configurable
type Factory interface {
	BaseFactory
	// Initialize performs internal initialization of the factory, such as opening connections to the backend store.
	// It is called after all configuration of the factory itself has been done.
	Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error
}

// Purger defines an interface that is capable of purging the storage.
// Only meant to be used from integration tests.
type Purger interface {
	// Purge removes all data from the storage.
	Purge(context.Context) error
}

// SamplingStoreFactory defines an interface that is capable of returning the necessary backends for
// adaptive sampling.
type SamplingStoreFactory interface {
	// CreateLock creates a distributed lock.
	CreateLock() (distributedlock.Lock, error)
	// CreateSamplingStore creates a sampling store.
	CreateSamplingStore(maxBuckets int) (samplingstore.Store, error)
}

// MetricStoreFactory defines an interface for a factory that can create implementations of different metrics storage components.
// Implementations are also encouraged to implement storage.Configurable interface.
//
// # See also
//
// storage.Configurable
type MetricStoreFactory interface {
	// Initialize performs internal initialization of the factory, such as opening connections to the backend store.
	// It is called after all configuration of the factory itself has been done.
	Initialize(telset telemetry.Settings) error

	// CreateMetricsReader creates a metricstore.Reader.
	CreateMetricsReader() (metricstore.Reader, error)
}

// Inheritable is an interface that can be implement by some storage implementations
// to provide a way to inherit configuration settings from another factory.
type Inheritable interface {
	InheritSettingsFrom(other Factory)
}

// ArchiveCapable is an interface that can be implemented by some storage implementations
// to indicate that they are capable of archiving data.
type ArchiveCapable interface {
	IsArchiveCapable() bool
}
