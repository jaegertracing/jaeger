// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
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

func makeStorageExtension(t *testing.T, memstoreName string) component.Host {
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}
	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.CreateExtension(
		context.Background(),
		extension.Settings{
			TelemetrySettings: telemetrySettings,
		},
		&jaegerstorage.Config{Backends: map[string]jaegerstorage.Backend{
			memstoreName: {Memory: &memory.Configuration{MaxTraces: 10000}},
		}},
	)
	require.NoError(t, err)

	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, storageExtension)

	err = storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(context.Background())) })
	return host
}

var _ component.Config = (*Config)(nil)

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
	storageHost := makeStorageExtension(t, "foobar")

	err = samplingExtension.Start(context.Background(), storageHost)
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

func TestGetAdaptiveSamplingComponents(t *testing.T) {
	// Success case
	host := makeRemoteSamplingExtension(t, &Config{
		Adaptive: &AdaptiveConfig{
			SamplingStore: "foobar",
			Options: adaptive.Options{
				FollowerLeaseRefreshInterval: 1,
				LeaderLeaseRefreshInterval:   1,
				AggregationBuckets:           1,
			},
		},
	})

	comps, err := GetAdaptiveSamplingComponents(host)
	require.NoError(t, err)
	require.NotNil(t, comps.DistLock)
	require.NotNil(t, comps.SamplingStore)
	require.Equal(t, comps.Options.FollowerLeaseRefreshInterval, time.Duration(1))
	require.Equal(t, comps.Options.LeaderLeaseRefreshInterval, time.Duration(1))
	require.Equal(t, comps.Options.AggregationBuckets, 1)

	// Error case
	host = makeRemoteSamplingExtension(t, &Config{})
	_, err = GetAdaptiveSamplingComponents(host)
	require.ErrorContains(t, err, "extension 'remote_sampling' is not configured for adaptive sampling")
}
