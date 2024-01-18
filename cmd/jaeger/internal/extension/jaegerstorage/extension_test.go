// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	grpcCfg "github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	memstoreName  = "memstore"
	grpcstoreName = "grpcstore"
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
	require.EqualError(t, err, fmt.Sprintf("%s: no storage type present in config", ID))
}

func TestStorageExtensionDuplicateNameError(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "memory",
			config: &Config{
				Memory: map[string]memoryCfg.Configuration{
					memstoreName: {},
				},
			},
		},
		{
			name: "grpc",
			config: &Config{
				GRPC: map[string]grpcCfg.Configuration{
					memstoreName: {},
				},
			},
		},
	}

	ctx := context.Background()
	ext := storageExt{
		factories: map[string]storage.Factory{
			memstoreName: memory.NewFactoryWithConfig(memoryCfg.Configuration{}, metrics.NullFactory, zap.NewNop()),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ext.config = test.config
			err := ext.Start(ctx, componenttest.NewNopHost())
			require.Error(t, err)
			require.EqualError(t, err, fmt.Sprintf("duplicate %s storage name %s", test.name, memstoreName))
		})
	}
	t.Cleanup(func() { require.NoError(t, ext.Shutdown(ctx)) })
}

func TestStorageFactoryBadHostError(t *testing.T) {
	makeStorageExtension(t, memstoreName)

	host := componenttest.NewNopHost()
	_, err := GetStorageFactory(memstoreName, host)
	require.Error(t, err)
	require.EqualError(t, err, fmt.Sprintf("cannot find extension '%s' (make sure it's defined earlier in the config)", ID))
}

func TestStorageFactoryBadNameError(t *testing.T) {
	storageExtension := makeStorageExtension(t, memstoreName)

	host := storageHost{t: t, storageExtension: storageExtension}
	const badMemstoreName = "test"

	_, err := GetStorageFactory(badMemstoreName, host)
	require.Error(t, err)
	require.EqualError(t, err, fmt.Sprintf("cannot find storage '%s' declared with '%s' extension", badMemstoreName, ID))
}

func TestStorageFactoryBadShutdownError(t *testing.T) {
	storageExtension := storageExt{
		factories: make(map[string]storage.Factory),
	}
	badFactoryError := fmt.Errorf("error factory")
	storageExtension.factories[memstoreName] = errorFactory{closeErr: badFactoryError}

	err := storageExtension.Shutdown(context.Background())
	require.ErrorIs(t, err, badFactoryError)
}

func TestStorageExtension(t *testing.T) {
	storageExtension := makeStorageExtension(t, memstoreName)

	host := storageHost{t: t, storageExtension: storageExtension}

	_, err := GetStorageFactory(memstoreName, host)
	require.NoError(t, err)
}

// This test is only to cover the grpc.NewFactoryWithConfig
// since it can't be mocked yet require external grpc connection
func TestGRPCStorageExtensionError(t *testing.T) {
	ctx := context.Background()
	ext := storageExt{
		config: &Config{
			GRPC: map[string]grpcCfg.Configuration{
				grpcstoreName: {},
			},
		},
		logger: zap.NewNop(),
		factories: map[string]storage.Factory{
			memstoreName: memory.NewFactoryWithConfig(memoryCfg.Configuration{}, metrics.NullFactory, zap.NewNop()),
		},
	}

	err := ext.Start(ctx, componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to initialize grpc storage: grpc-plugin builder failed to create a store: error connecting to remote storage")
}

func makeStorageExtension(t *testing.T, memstoreName string) component.Component {
	extensionFactory := NewFactory()

	ctx := context.Background()
	telemetrySettings := component.TelemetrySettings{
		Logger:         zap.L(),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}
	config := &Config{
		Memory: map[string]memoryCfg.Configuration{
			memstoreName: {MaxTraces: 10000},
		},
	}
	err := config.Validate()
	require.NoError(t, err)

	storageExtension, err := extensionFactory.CreateExtension(ctx, extension.CreateSettings{
		ID:                ID,
		TelemetrySettings: telemetrySettings,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}, config)
	require.NoError(t, err)

	host := componenttest.NewNopHost()
	err = storageExtension.Start(ctx, host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(ctx)) })

	return storageExtension
}
