// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	es "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// CreateStorageFactory creates a trace and dependency store factory from the storage configuration.
// This reuses the factory creation logic from jaegerstorage extension but adapted for standalone use.
func CreateStorageFactory(
	ctx context.Context,
	storageName string,
	cfg *StorageConfig,
	telset telemetry.Settings,
) (tracestore.Factory, depstore.Factory, error) {
	backend, ok := cfg.Backends[storageName]
	if !ok {
		return nil, nil, fmt.Errorf("storage backend '%s' not found in configuration", storageName)
	}

	telset.Logger.Info("Initializing storage", zap.String("name", storageName))
	telset.Metrics = telset.Metrics.Namespace(metrics.NSOptions{
		Name: "storage",
		Tags: map[string]string{
			"name": storageName,
			"role": "tracestore",
		},
	})

	var factory tracestore.Factory
	var err error

	switch {
	case backend.Memory != nil:
		factory, err = memory.NewFactory(*backend.Memory, telset)
	case backend.Badger != nil:
		factory, err = badger.NewFactory(*backend.Badger, telset.Metrics, telset.Logger)
	case backend.GRPC != nil:
		factory, err = grpc.NewFactory(ctx, *backend.GRPC, telset)
	case backend.Cassandra != nil:
		factory, err = cassandra.NewFactory(*backend.Cassandra, telset.Metrics, telset.Logger)
	case backend.Elasticsearch != nil:
		factory, err = es.NewFactory(ctx, *backend.Elasticsearch, telset, nil)
	case backend.Opensearch != nil:
		factory, err = es.NewFactory(ctx, *backend.Opensearch, telset, nil)
	case backend.ClickHouse != nil:
		factory, err = clickhouse.NewFactory(ctx, *backend.ClickHouse, telset)
	default:
		err = errors.New("empty configuration")
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize storage '%s': %w", storageName, err)
	}

	depFactory, ok := factory.(depstore.Factory)
	if !ok {
		return nil, nil, fmt.Errorf("storage '%s' does not implement dependency store", storageName)
	}

	return factory, depFactory, nil
}
