// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func getTelemetrySettings() telemetry.Settings {
	return telemetry.Settings{
		Logger:        zap.NewNop(),
		Metrics:       metrics.NullFactory,
		MeterProvider: noop.NewMeterProvider(),
	}
}

func TestCreateTraceStorageFactory_Memory(t *testing.T) {
	backend := TraceBackend{
		Memory: &memory.Configuration{
			MaxTraces: 10000,
		},
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"memory-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
}

func TestCreateTraceStorageFactory_Badger(t *testing.T) {
	backend := TraceBackend{
		Badger: &badger.Config{
			Ephemeral:             true,
			MaintenanceInterval:   5,
			MetricsUpdateInterval: 10,
		},
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"badger-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
}

func TestCreateTraceStorageFactory_GRPC(t *testing.T) {
	t.Skip("GRPC factory requires complex setup and proper gRPC server - tested in integration tests")
	cfg := grpc.DefaultConfig()
	cfg.ClientConfig.Endpoint = "localhost:17271"
	backend := TraceBackend{
		GRPC: &cfg,
	}

	_, err := CreateTraceStorageFactory(
		context.Background(),
		"grpc-test",
		backend,
		getTelemetrySettings(),
		nil,
	)
	if err != nil {
		assert.Contains(t, err.Error(), "grpc-test")
	}
}

func TestCreateTraceStorageFactory_Cassandra(t *testing.T) {
	backend := TraceBackend{
		Cassandra: &cassandra.Options{},
	}

	_, err := CreateTraceStorageFactory(
		context.Background(),
		"cassandra-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	// Cassandra will fail without proper servers config, but we're testing the factory creation path
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize storage 'cassandra-test'")
}

func TestCreateTraceStorageFactory_Elasticsearch(t *testing.T) {
	t.Skip("Elasticsearch factory tries to connect to ES instance - tested in integration tests")
}

func TestCreateTraceStorageFactory_ElasticsearchWithAuthResolver(t *testing.T) {
	t.Skip("Elasticsearch factory tries to connect to ES instance - tested in integration tests")
}

func TestCreateTraceStorageFactory_Opensearch(t *testing.T) {
	t.Skip("OpenSearch factory tries to connect to OS instance - tested in integration tests")
}

func TestCreateTraceStorageFactory_OpensearchWithAuthResolver(t *testing.T) {
	t.Skip("OpenSearch factory tries to connect to OS instance - tested in integration tests")
}

func TestCreateTraceStorageFactory_ClickHouse(t *testing.T) {
	backend := TraceBackend{
		ClickHouse: &clickhouse.Configuration{},
	}

	_, err := CreateTraceStorageFactory(
		context.Background(),
		"clickhouse-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	// ClickHouse will fail without proper config, but we're testing the factory creation path
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize storage 'clickhouse-test'")
}

func TestCreateTraceStorageFactory_EmptyBackend(t *testing.T) {
	backend := TraceBackend{}

	_, err := CreateTraceStorageFactory(
		context.Background(),
		"empty-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize storage 'empty-test'")
	assert.Contains(t, err.Error(), "empty configuration")
}

func TestCreateTraceStorageFactory_AuthResolverError(t *testing.T) {
	backend := TraceBackend{
		Elasticsearch: &escfg.Configuration{
			Servers:  []string{"http://localhost:9200"},
			LogLevel: "error",
		},
	}

	authResolver := func(authCfg escfg.Authentication, backendType, backendName string) (extensionauth.HTTPClient, error) {
		return nil, errors.New("auth error")
	}

	_, err := CreateTraceStorageFactory(
		context.Background(),
		"es-test",
		backend,
		getTelemetrySettings(),
		authResolver,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auth error")
}
