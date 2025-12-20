// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

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
type MetricStoreFactory interface {
	CreateMetricsReader() (metricstore.Reader, error)
}

// V1MetricStoreFactory is a v1 version of MetricStoreFactory.
// Implementations are encouraged to implement storage.Configurable interface.
//
// # See also
//
// storage.Configurable
type V1MetricStoreFactory interface {
	MetricStoreFactory
	// Initialize performs internal initialization of the factory, such as opening connections to the backend store.
	// It is called after all configuration of the factory itself has been done.
	Initialize(telset telemetry.Settings) error
}

// ArchiveCapable is an interface that can be implemented by some storage implementations
// to indicate that they are capable of archiving data.
type ArchiveCapable interface {
	IsArchiveCapable() bool
}
