// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageconfig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func getTelemetrySettings() telemetry.Settings {
	return telemetry.Settings{
		Logger:        zap.NewNop(),
		Metrics:       metrics.NullFactory,
		MeterProvider: noop.NewMeterProvider(),
		Host:          componenttest.NewNopHost(),
	}
}

func setupMockServer(t *testing.T, response []byte, statusCode int) *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(response)
	}))
	require.NotNil(t, mockServer)
	t.Cleanup(mockServer.Close)
	return mockServer
}

func getVersionResponse(t *testing.T) []byte {
	versionResponse, e := json.Marshal(map[string]any{
		"Version": map[string]any{
			"Number": "7",
		},
	})
	require.NoError(t, e)
	return versionResponse
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
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
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
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
}

func TestCreateTraceStorageFactory_GRPC(t *testing.T) {
	backend := TraceBackend{
		GRPC: &grpc.Config{
			ClientConfig: configgrpc.ClientConfig{
				Endpoint: "localhost:12345",
			},
		},
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"grpc-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to initialize storage 'cassandra-test'")
}

func TestCreateTraceStorageFactory_Elasticsearch(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	backend := TraceBackend{
		Elasticsearch: &escfg.Configuration{
			Servers:  []string{server.URL},
			LogLevel: "error",
		},
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"es-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
}

func TestCreateTraceStorageFactory_ElasticsearchWithAuthResolver(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	backend := TraceBackend{
		Elasticsearch: &escfg.Configuration{
			Servers:  []string{server.URL},
			LogLevel: "error",
		},
	}

	authResolver := func(_ escfg.Authentication, _, _ string) (extensionauth.HTTPClient, error) {
		return nil, nil // No auth needed for this test
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"es-test",
		backend,
		getTelemetrySettings(),
		authResolver,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
}

func TestCreateTraceStorageFactory_ElasticsearchAuthResolverError(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	backend := TraceBackend{
		Elasticsearch: &escfg.Configuration{
			Servers:  []string{server.URL},
			LogLevel: "error",
		},
	}

	authResolver := func(_ escfg.Authentication, _, _ string) (extensionauth.HTTPClient, error) {
		return nil, errors.New("auth error")
	}

	_, err := CreateTraceStorageFactory(
		context.Background(),
		"es-test",
		backend,
		getTelemetrySettings(),
		authResolver,
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "auth error")
}

func TestCreateTraceStorageFactory_Opensearch(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	backend := TraceBackend{
		Opensearch: &escfg.Configuration{
			Servers:  []string{server.URL},
			LogLevel: "error",
		},
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"os-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
}

func TestCreateTraceStorageFactory_OpensearchWithAuthResolver(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	backend := TraceBackend{
		Opensearch: &escfg.Configuration{
			Servers:  []string{server.URL},
			LogLevel: "error",
		},
	}

	authResolver := func(_ escfg.Authentication, _, _ string) (extensionauth.HTTPClient, error) {
		return nil, nil // No auth needed for this test
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"os-test",
		backend,
		getTelemetrySettings(),
		authResolver,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
}

func TestCreateTraceStorageFactory_OpensearchAuthResolverError(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	backend := TraceBackend{
		Opensearch: &escfg.Configuration{
			Servers:  []string{server.URL},
			LogLevel: "error",
		},
	}

	authResolver := func(_ escfg.Authentication, _, _ string) (extensionauth.HTTPClient, error) {
		return nil, errors.New("auth error for opensearch")
	}

	_, err := CreateTraceStorageFactory(
		context.Background(),
		"os-test",
		backend,
		getTelemetrySettings(),
		authResolver,
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "auth error for opensearch")
}

func TestCreateTraceStorageFactory_ClickHouse(t *testing.T) {
	testServer := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	t.Cleanup(testServer.Close)

	backend := TraceBackend{
		ClickHouse: &clickhouse.Configuration{
			Protocol: "http",
			Addresses: []string{
				testServer.Listener.Addr().String(),
			},
		},
	}

	factory, err := CreateTraceStorageFactory(
		context.Background(),
		"clickhouse-test",
		backend,
		getTelemetrySettings(),
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, factory)
	t.Cleanup(func() {
		if closer, ok := factory.(io.Closer); ok {
			require.NoError(t, closer.Close())
		}
	})
}

func TestCreateTraceStorageFactory_ClickHouseError(t *testing.T) {
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to initialize storage 'clickhouse-test'")
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

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to initialize storage 'empty-test'")
	require.Contains(t, err.Error(), "empty configuration")
}
