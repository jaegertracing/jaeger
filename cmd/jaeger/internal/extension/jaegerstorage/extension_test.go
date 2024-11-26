// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	esCfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	promCfg "github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type errorFactory struct {
	closeErr error
}

func (errorFactory) Initialize(metrics.Factory, *zap.Logger) error {
	panic("not implemented")
}

func (errorFactory) CreateSpanReader() (spanstore.Reader, error) {
	panic("not implemented")
}

func (errorFactory) CreateSpanWriter() (spanstore.Writer, error) {
	panic("not implemented")
}

func (errorFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	panic("not implemented")
}

func (e errorFactory) Close() error {
	return e.closeErr
}

func TestStorageFactoryBadHostError(t *testing.T) {
	_, err := GetStorageFactory("something", componenttest.NewNopHost())
	require.ErrorContains(t, err, "cannot find extension")
}

func TestStorageFactoryBadNameError(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(ID, startStorageExtension(t, "foo", ""))
	_, err := GetStorageFactory("bar", host)
	require.ErrorContains(t, err, "cannot find definition of storage 'bar'")
}

func TestMetricsFactoryBadHostError(t *testing.T) {
	_, err := GetMetricStorageFactory("something", componenttest.NewNopHost())
	require.ErrorContains(t, err, "cannot find extension")
}

func TestMetricsFactoryBadNameError(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(ID, startStorageExtension(t, "", "foo"))
	_, err := GetMetricStorageFactory("bar", host)
	require.ErrorContains(t, err, "cannot find metric storage 'bar'")
}

func TestStorageExtensionType(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(ID, startStorageExtension(t, "", "foo"))
	_, err := findExtension(host)
	require.NoError(t, err)
}

func TestStorageFactoryBadShutdownError(t *testing.T) {
	shutdownError := errors.New("shutdown error")
	ext := storageExt{
		factories: map[string]storage.Factory{
			"foo": errorFactory{closeErr: shutdownError},
		},
	}
	err := ext.Shutdown(context.Background())
	require.ErrorIs(t, err, shutdownError)
}

func TestGetFactoryV2Error(t *testing.T) {
	host := componenttest.NewNopHost()
	_, err := GetStorageFactoryV2("something", host)
	require.ErrorContains(t, err, "cannot find extension")
}

func TestGetFactory(t *testing.T) {
	const name = "foo"
	const metricname = "bar"
	host := storagetest.NewStorageHost().WithExtension(ID, startStorageExtension(t, name, metricname))
	f, err := GetStorageFactory(name, host)
	require.NoError(t, err)
	require.NotNil(t, f)

	f2, err := GetStorageFactoryV2(name, host)
	require.NoError(t, err)
	require.NotNil(t, f2)

	f3, err := GetMetricStorageFactory(metricname, host)
	require.NoError(t, err)
	require.NotNil(t, f3)
}

func TestBadger(t *testing.T) {
	ext := makeStorageExtenion(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"foo": {
				Badger: &badger.Config{
					Ephemeral:             true,
					MaintenanceInterval:   5,
					MetricsUpdateInterval: 10,
				},
			},
		},
	})
	ctx := context.Background()
	err := ext.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(ctx))
}

func TestGRPC(t *testing.T) {
	ext := makeStorageExtenion(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"foo": {
				GRPC: &grpc.Config{
					ClientConfig: configgrpc.ClientConfig{
						Endpoint: "localhost:12345",
					},
				},
			},
		},
	})
	ctx := context.Background()
	err := ext.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(ctx))
}

func TestPrometheus(t *testing.T) {
	ext := makeStorageExtenion(t, &Config{
		MetricBackends: map[string]MetricBackend{
			"foo": {
				Prometheus: &promCfg.Configuration{
					ServerURL: "localhost:12345",
				},
			},
		},
	})
	ctx := context.Background()
	err := ext.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(ctx))
}

func TestStartError(t *testing.T) {
	ext := makeStorageExtenion(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"foo": {},
		},
	})
	err := ext.Start(context.Background(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "failed to initialize storage 'foo'")
	require.ErrorContains(t, err, "empty configuration")
}

func TestMetricsStorageStartError(t *testing.T) {
	ext := makeStorageExtenion(t, &Config{
		MetricBackends: map[string]MetricBackend{
			"foo": {
				Prometheus: &promCfg.Configuration{},
			},
		},
	})
	err := ext.Start(context.Background(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "failed to initialize metrics storage 'foo'")
}

func testElasticsearchOrOpensearch(t *testing.T, cfg TraceBackend) {
	ext := makeStorageExtenion(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"foo": cfg,
		},
	})
	ctx := context.Background()
	err := ext.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(ctx))
}

func TestXYZsearch(t *testing.T) {
	versionResponse, err := json.Marshal(map[string]any{
		"Version": map[string]any{
			"Number": "7",
		},
	})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(versionResponse)
	}))
	defer server.Close()
	t.Run("Elasticsearch", func(t *testing.T) {
		testElasticsearchOrOpensearch(t, TraceBackend{
			Elasticsearch: &esCfg.Configuration{
				Servers:  []string{server.URL},
				LogLevel: "error",
			},
		})
	})
	t.Run("OpenSearch", func(t *testing.T) {
		testElasticsearchOrOpensearch(t, TraceBackend{
			Opensearch: &esCfg.Configuration{
				Servers:  []string{server.URL},
				LogLevel: "error",
			},
		})
	})
}

func TestCassandraError(t *testing.T) {
	// since we cannot successfully create storage factory for Cassandra
	// without running a Cassandra server, we only test the error case.
	ext := makeStorageExtenion(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"cassandra": {
				Cassandra: &cassandra.Options{},
			},
		},
	})
	err := ext.Start(context.Background(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "failed to initialize storage 'cassandra'")
	require.ErrorContains(t, err, "Servers: non zero value required")
}

func noopTelemetrySettings() component.TelemetrySettings {
	return component.TelemetrySettings{
		Logger:         zap.L(),
		TracerProvider: nooptrace.NewTracerProvider(),
		LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
			return noopmetric.NewMeterProvider()
		},
		MeterProvider: noopmetric.NewMeterProvider(),
	}
}

func makeStorageExtenion(t *testing.T, config *Config) component.Component {
	extensionFactory := NewFactory()
	ctx := context.Background()
	ext, err := extensionFactory.Create(ctx,
		extension.Settings{
			ID:                ID,
			TelemetrySettings: noopTelemetrySettings(),
			BuildInfo:         component.NewDefaultBuildInfo(),
		},
		config,
	)
	require.NoError(t, err)
	return ext
}

func startStorageExtension(t *testing.T, memstoreName string, promstoreName string) component.Component {
	config := &Config{
		TraceBackends: map[string]TraceBackend{
			memstoreName: {
				Memory: &memory.Configuration{
					MaxTraces: 10000,
				},
			},
		},
		MetricBackends: map[string]MetricBackend{
			promstoreName: {
				Prometheus: &promCfg.Configuration{
					ServerURL: "localhost:12345",
				},
			},
		},
	}
	require.NoError(t, config.Validate())

	ext := makeStorageExtenion(t, config)
	err := ext.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, ext.Shutdown(context.Background()))
	})
	return ext
}
