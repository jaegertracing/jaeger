// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
)

type storageHost struct {
	t                *testing.T
	storageExtension component.Component
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		jaegerstorage.ID: host.storageExtension,
	}
}

func (host storageHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory { return nil }
func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

type samplingHost struct {
	t                 *testing.T
	samplingExtension component.Component
}

func (host samplingHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		ID: host.samplingExtension,
	}
}

func (host samplingHost) ReportFatalError(err error) {
	host.t.Fatal(err)
}

func (samplingHost) GetFactory(_ component.Kind, _ component.Type) component.Factory { return nil }
func (samplingHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

func makeStorageExtension(t *testing.T, memstoreName string) storageHost {
	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.CreateExtension(
		context.Background(),
		extension.Settings{
			TelemetrySettings: component.TelemetrySettings{
				Logger:         zap.L(),
				TracerProvider: nooptrace.NewTracerProvider(),
			},
		},
		&jaegerstorage.Config{Memory: map[string]memoryCfg.Configuration{
			memstoreName: {MaxTraces: 10000},
		}})
	require.NoError(t, err)
	host := storageHost{t: t, storageExtension: storageExtension}

	err = storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(context.Background())) })
	return host
}

func makeRemoteSamplingExtension(t *testing.T, cfg component.Config) samplingHost {
	extensionFactory := NewFactory()
	samplingExtension, err := extensionFactory.CreateExtension(
		context.Background(),
		extension.Settings{
			TelemetrySettings: component.TelemetrySettings{
				Logger:         zap.L(),
				TracerProvider: nooptrace.NewTracerProvider(),
			},
		},
		cfg,
	)
	require.NoError(t, err)
	host := samplingHost{t: t, samplingExtension: samplingExtension}

	err = samplingExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, samplingExtension.Shutdown(context.Background())) })
	return host
}

func TestStartFileBasedProvider(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.File.Path = filepath.Join("..", "..", "..", "sampling-strategies.json")
	cfg.Adaptive = nil
	cfg.HTTP = nil
	cfg.GRPC = nil
	require.NoError(t, cfg.Validate())

	ext, err := factory.CreateExtension(context.Background(), extension.Settings{
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	host := makeStorageExtension(t, "foobar")
	require.NoError(t, ext.Start(context.Background(), host))
	require.NoError(t, ext.Shutdown(context.Background()))
}

func TestStartAdaptiveProvider(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.File = nil
	cfg.Adaptive.SamplingStore = "foobar"
	cfg.HTTP = nil
	cfg.GRPC = nil
	require.NoError(t, cfg.Validate())

	ext, err := factory.CreateExtension(context.Background(), extension.Settings{
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	host := makeStorageExtension(t, "foobar")
	require.NoError(t, ext.Start(context.Background(), host))
	require.NoError(t, ext.Shutdown(context.Background()))
}

// storageExtension, err := extensionFactory.CreateExtension(
// 	context.Background(),
// 	extension.Settings{
// 		TelemetrySettings: component.TelemetrySettings{
// 			Logger:         zap.L(),
// 			TracerProvider: nooptrace.NewTracerProvider(),
// 		},
// 	},
// 	&jaegerstorage.Config{Memory: map[string]memoryCfg.Configuration{
// 		memstoreName: {MaxTraces: 10000},
// 	}})

func TestGetAdaptiveSamplingComponents(t *testing.T) {
	host := makeStorageExtension(t, "foobar")
	_, err := GetAdaptiveSamplingComponents(host)
	require.Error(t, err)

	samplingG := makeRemoteSamplingExtension(t, &Config{})
	_, err = GetAdaptiveSamplingComponents(samplingG)
	require.NoError(t, err)

	// host = makeRemoteSamplingExtension(t, &Config{})
	// t.Log(host.GetExtensions())
	// _, err = GetAdaptiveSamplingComponents(host)
	// require.NoError(t, err)

	// tests := []struct {
	// 	name          string
	// 	setupHost     func(*testing.T) component.Host
	// 	expectedError string
	// }{
	// 	{
	// 		name: "successful retrieval",
	// 		setupHost: func(t *testing.T) component.Host {
	// 			factory := NewFactory()
	// 			cfg := factory.CreateDefaultConfig().(*Config)
	// 			cfg.File = nil
	// 			cfg.Adaptive.SamplingStore = "foobar"
	// 			cfg.HTTP = nil
	// 			cfg.GRPC = nil
	// 			require.NoError(t, cfg.Validate())

	// 			ext, err := factory.CreateExtension(context.Background(), extension.Settings{
	// 				TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	// 			}, cfg)
	// 			require.NoError(t, err)
	// 			host := makeStorageExtension(t, "foobar")
	// 			require.NoError(t, ext.Start(context.Background(), host))
	// 			t.Cleanup(func() {
	// 				require.NoError(t, ext.Shutdown(context.Background()))
	// 			})
	// 			return host
	// 		},
	// 		expectedError: "",
	// 	},
	// 	{
	// 		name: "extension not found",
	// 		setupHost: func(t *testing.T) component.Host {
	// 			return storageHost{t: t}
	// 		},
	// 		expectedError: "cannot find extension 'jaeger.remote_sampling' (make sure it's defined earlier in the config)",
	// 	},
	// 	// {
	// 	// 	name: "incorrect extension type",
	// 	// 	setupHost: func(t *testing.T) component.Host {
	// 	// 		host := storageHost{t: t}
	// 	// 		host.storageExtension = componenttest.NewNopExtension()
	// 	// 		return host
	// 	// 	},
	// 	// 	expectedError: "extension 'jaeger.remote_sampling' is not of type 'jaeger.remote_sampling'",
	// 	// },
	// }

	// for _, tt := range tests {
	// 	t.Run(tt.name, func(t *testing.T) {
	// 		host := tt.setupHost(t)
	// 		components, err := GetAdaptiveSamplingComponents(host)

	// 		if tt.expectedError != "" {
	// 			require.Error(t, err)
	// 			require.Contains(t, err.Error(), tt.expectedError)
	// 			require.Nil(t, components)
	// 		} else {
	// 			require.NoError(t, err)
	// 			require.NotNil(t, components)
	// 			require.NotNil(t, components.SamplingStore)
	// 			require.NotNil(t, components.DistLock)
	// 			require.NotNil(t, components.Options)
	// 		}
	// 	})
	// }
}
