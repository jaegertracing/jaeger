// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	esCfg "github.com/jaegertracing/jaeger/pkg/es/config"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	badgerCfg "github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type storageHost struct {
	t                *testing.T
	storageExtension component.Component
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		ID: host.storageExtension,
	}
}

func (host storageHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	return nil
}

func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

type errorFactory struct {
	closeErr error
}

func (e errorFactory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	panic("not implemented")
}

func (e errorFactory) CreateSpanReader() (spanstore.Reader, error) {
	panic("not implemented")
}

func (e errorFactory) CreateSpanWriter() (spanstore.Writer, error) {
	panic("not implemented")
}

func (e errorFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	panic("not implemented")
}

func (e errorFactory) Close() error {
	return e.closeErr
}

func TestStorageExtensionConfigError(t *testing.T) {
	config := createDefaultConfig().(*Config)
	err := config.Validate()
	require.ErrorContains(t, err, "no storage type present in config")
}

func TestStorageExtensionNameConflict(t *testing.T) {
	storageExtension := makeStorageExtenion(t, &Config{
		Memory: map[string]memoryCfg.Configuration{
			"foo": {MaxTraces: 10000},
		},
		Badger: map[string]badgerCfg.NamespaceConfig{
			"foo": {},
		},
	})
	err := storageExtension.Start(context.Background(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "duplicate")
}

func TestStorageFactoryBadHostError(t *testing.T) {
	host := componenttest.NewNopHost()
	_, err := GetStorageFactory("something", host)
	require.ErrorContains(t, err, "cannot find extension")
}

func TestStorageFactoryBadNameError(t *testing.T) {
	host := storageHost{t: t, storageExtension: startStorageExtension(t, "foo")}
	_, err := GetStorageFactory("bar", host)
	require.ErrorContains(t, err, "cannot find storage 'bar'")
}

func TestStorageFactoryBadShutdownError(t *testing.T) {
	shutdownError := fmt.Errorf("shutdown error")
	storageExtension := storageExt{
		factories: map[string]storage.Factory{
			"foo": errorFactory{closeErr: shutdownError},
		},
	}
	err := storageExtension.Shutdown(context.Background())
	require.ErrorIs(t, err, shutdownError)
}

func TestStorageExtension(t *testing.T) {
	const name = "foo"
	host := storageHost{t: t, storageExtension: startStorageExtension(t, name)}
	f, err := GetStorageFactory(name, host)
	require.NoError(t, err)
	require.NotNil(t, f)
}

func TestBadgerStorageExtension(t *testing.T) {
	storageExtension := makeStorageExtenion(t, &Config{
		Badger: map[string]badgerCfg.NamespaceConfig{
			"foo": {
				Ephemeral:             true,
				MaintenanceInterval:   5,
				MetricsUpdateInterval: 10,
			},
		},
	})
	ctx := context.Background()
	err := storageExtension.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, storageExtension.Shutdown(ctx))
}

func TestBadgerStorageExtensionError(t *testing.T) {
	ext := makeStorageExtenion(t, &Config{
		Badger: map[string]badgerCfg.NamespaceConfig{
			"foo": {
				KeyDirectory:   "/bad/path",
				ValueDirectory: "/bad/path",
			},
		},
	})
	err := ext.Start(context.Background(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "failed to initialize badger storage")
	require.ErrorContains(t, err, "/bad/path")
}

func TestESStorageExtension(t *testing.T) {
	mockEsServerResponse := []byte(`
	{
		"Version": {
			"Number": "6"
		}
	}
	`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()
	storageExtension := makeStorageExtenion(t, &Config{
		Elasticsearch: map[string]esCfg.Configuration{
			"foo": {
				Servers:  []string{server.URL},
				LogLevel: "error",
			},
		},
	})
	ctx := context.Background()
	err := storageExtension.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, storageExtension.Shutdown(ctx))
}

func TestESStorageExtensionError(t *testing.T) {
	defer testutils.VerifyGoLeaksOnce(t)

	ext := makeStorageExtenion(t, &Config{
		Elasticsearch: map[string]esCfg.Configuration{
			"foo": {
				Servers:  []string{"http://127.0.0.1:65535"},
				LogLevel: "error",
			},
		},
	})
	err := ext.Start(context.Background(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "failed to initialize elasticsearch storage")
	require.ErrorContains(t, err, "http://127.0.0.1:65535")
}

func noopTelemetrySettings() component.TelemetrySettings {
	return component.TelemetrySettings{
		Logger:         zap.L(),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}
}

func makeStorageExtenion(t *testing.T, config *Config) component.Component {
	extensionFactory := NewFactory()
	ctx := context.Background()
	storageExtension, err := extensionFactory.CreateExtension(ctx,
		extension.CreateSettings{
			ID:                ID,
			TelemetrySettings: noopTelemetrySettings(),
			BuildInfo:         component.NewDefaultBuildInfo(),
		},
		config,
	)
	require.NoError(t, err)
	return storageExtension
}

func startStorageExtension(t *testing.T, memstoreName string) component.Component {
	config := &Config{
		Memory: map[string]memoryCfg.Configuration{
			memstoreName: {MaxTraces: 10000},
		},
	}
	require.NoError(t, config.Validate())

	storageExtension := makeStorageExtenion(t, config)
	err := storageExtension.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, storageExtension.Shutdown(context.Background()))
	})
	return storageExtension
}
