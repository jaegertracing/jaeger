// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageconfig

import (
	"context"
	"errors"
	"fmt"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	es "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// CreateStorageFactory creates a trace storage factory from the backend configuration.
// This is extracted from jaegerstorage extension to be shared between jaeger and remote-storage.
func CreateStorageFactory(
	ctx context.Context,
	name string,
	backend TraceBackend,
	telset telemetry.Settings,
) (tracestore.Factory, error) {
	telset.Logger.Sugar().Infof("Initializing storage '%s'", name)

	// Create scoped metrics factory
	telset.Metrics = telset.Metrics.Namespace(metrics.NSOptions{
		Name: "storage",
		Tags: map[string]string{
			"name": name,
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
		// Note: httpAuth is nil for remote-storage since it doesn't have access to auth extensions
		factory, err = es.NewFactory(ctx, *backend.Elasticsearch, telset, nil)
	case backend.Opensearch != nil:
		factory, err = es.NewFactory(ctx, *backend.Opensearch, telset, nil)
	case backend.ClickHouse != nil:
		factory, err = clickhouse.NewFactory(ctx, *backend.ClickHouse, telset)
	default:
		err = errors.New("empty configuration")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage '%s': %w", name, err)
	}

	return factory, nil
}
