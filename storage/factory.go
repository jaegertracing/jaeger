// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/metricstore"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
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
// Implementations are also encouraged to implement plugin.Configurable interface.
//
// # See also
//
// plugin.Configurable
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

var (
	// ErrArchiveStorageNotConfigured can be returned by the ArchiveFactory when the archive storage is not configured.
	ErrArchiveStorageNotConfigured = errors.New("archive storage not configured")

	// ErrArchiveStorageNotSupported can be returned by the ArchiveFactory when the archive storage is not supported by the backend.
	ErrArchiveStorageNotSupported = errors.New("archive storage not supported")
)

// ArchiveFactory is an additional interface that can be implemented by a factory to support trace archiving.
type ArchiveFactory interface {
	// CreateArchiveSpanReader creates a spanstore.Reader.
	CreateArchiveSpanReader() (spanstore.Reader, error)

	// CreateArchiveSpanWriter creates a spanstore.Writer.
	CreateArchiveSpanWriter() (spanstore.Writer, error)
}

// MetricStoreFactory defines an interface for a factory that can create implementations of different metrics storage components.
// Implementations are also encouraged to implement plugin.Configurable interface.
//
// # See also
//
// plugin.Configurable
type MetricStoreFactory interface {
	// Initialize performs internal initialization of the factory, such as opening connections to the backend store.
	// It is called after all configuration of the factory itself has been done.
	Initialize(telset telemetry.Settings) error

	// CreateMetricsReader creates a metricstore.Reader.
	CreateMetricsReader() (metricstore.Reader, error)
}
