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
)

type storageHost struct {
	t                *testing.T
	storageExtension component.Component
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	myMap := make(map[component.ID]component.Component)
	myMap[ID] = host.storageExtension
	return myMap
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

func TestExtensionConfigError(t *testing.T) {
	config := createDefaultConfig().(*Config)
	err := config.Validate()
	require.EqualError(t, err, "no storage type present in config")
}

func TestStartStorageExtensionError(t *testing.T) {
	ctx := context.Background()
	const memstoreName = "memstore"

	storageExtension := makeStorageExtension(t, memstoreName)

	host := componenttest.NewNopHost()
	err := storageExtension.Start(ctx, host)
	require.Error(t, err)
	require.EqualError(t, err, fmt.Sprintf("duplicate memory storage name %s", memstoreName))
}

func TestGetStorageFactoryError(t *testing.T) {
	const memstoreName = "memstore"

	makeStorageExtension(t, memstoreName)

	host := componenttest.NewNopHost()
	_, err := GetStorageFactory(memstoreName, host)
	require.Error(t, err)
	require.EqualError(t, err, fmt.Sprintf("cannot find extension '%s' (make sure it's defined earlier in the config)", ID))
}

func TestStorageExtension(t *testing.T) {
	const memstoreName = "memstore"

	storageExtension := makeStorageExtension(t, memstoreName)

	host := storageHost{t: t, storageExtension: storageExtension}

	_, err := GetStorageFactory(memstoreName, host)
	require.NoError(t, err)
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
