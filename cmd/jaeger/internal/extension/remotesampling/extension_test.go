// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
)

func makeStorageExtension(t *testing.T, memstoreName string) component.Host {
	telemetrySettings := component.TelemetrySettings{
		Logger:         zaptest.NewLogger(t),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}
	extensionFactory := jaegerstorage.NewFactory()
	storageExtension, err := extensionFactory.Create(
		context.Background(),
		extension.Settings{
			ID:                jaegerstorage.ID,
			TelemetrySettings: telemetrySettings,
		},
		&jaegerstorage.Config{TraceBackends: map[string]jaegerstorage.TraceBackend{
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

func makeRemoteSamplingExtension(t *testing.T, cfg component.Config) component.Host {
	extensionFactory := NewFactory()
	samplingExtension, err := extensionFactory.Create(
		context.Background(),
		extension.Settings{
			ID: ID,
			TelemetrySettings: component.TelemetrySettings{
				Logger:         zap.L(),
				TracerProvider: nooptrace.NewTracerProvider(),
			},
		},
		cfg,
	)
	require.NoError(t, err)
	host := storagetest.NewStorageHost().WithExtension(ID, samplingExtension)
	storageHost := makeStorageExtension(t, "foobar")

	require.NoError(t, samplingExtension.Start(context.Background(), storageHost))
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

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	host := makeStorageExtension(t, "foobar")
	require.NoError(t, ext.Start(context.Background(), host))
	require.NoError(t, ext.Shutdown(context.Background()))
}

func TestStartHTTP(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.File.Path = filepath.Join("..", "..", "..", "sampling-strategies.json")
	cfg.Adaptive = nil
	cfg.HTTP = &confighttp.ServerConfig{
		Endpoint: "0.0.0.0:12345",
	}
	cfg.GRPC = nil
	require.NoError(t, cfg.Validate())

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	// here, the file is using a service hardcoded in the sampling-services.json file
	host := makeStorageExtension(t, "foobar")
	require.NoError(t, ext.Start(context.Background(), host))

	resp, err := http.Get("http://0.0.0.0:12345/api/sampling?service=foo")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	expectedResponse := `{
        "probabilisticSampling": {
            "samplingRate": 0.8
        },
        "strategyType": 0
    }`
	require.JSONEq(t, expectedResponse, string(body))

	require.NoError(t, ext.Shutdown(context.Background()))
}

func TestStartGRPC(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.File.Path = filepath.Join("..", "..", "..", "sampling-strategies.json")
	cfg.Adaptive = nil
	cfg.HTTP = nil
	cfg.GRPC = &configgrpc.ServerConfig{
		NetAddr: confignet.AddrConfig{
			Endpoint:  "0.0.0.0:12346",
			Transport: "tcp",
		},
	}
	require.NoError(t, cfg.Validate())

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	// here, the file is using a service hardcoded in the sampling-services.json file
	host := makeStorageExtension(t, "foobar")
	require.NoError(t, ext.Start(context.Background(), host))

	conn, err := grpc.NewClient("0.0.0.0:12346", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	c := api_v2.NewSamplingManagerClient(conn)
	response, err := c.GetSamplingStrategy(context.Background(), &api_v2.SamplingStrategyParameters{ServiceName: "foo"})
	require.NoError(t, err)
	require.InDelta(t, 0.8, response.ProbabilisticSampling.SamplingRate, 0.01)

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

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	host := makeStorageExtension(t, "foobar")
	require.NoError(t, ext.Start(context.Background(), host))
	require.NoError(t, ext.Shutdown(context.Background()))
}

func TestStartAdaptiveStrategyProviderErrors(t *testing.T) {
	host := storagetest.NewStorageHost()
	ext := &rsExtension{
		cfg: &Config{
			Adaptive: &AdaptiveConfig{
				SamplingStore: "foobar",
			},
		},
	}
	err := ext.startAdaptiveStrategyProvider(host)
	require.ErrorContains(t, err, "failed to obtain sampling store factory")
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
	assert.NotNil(t, comps.DistLock)
	assert.NotNil(t, comps.SamplingStore)
	assert.Equal(t, time.Duration(1), comps.Options.FollowerLeaseRefreshInterval)
	assert.Equal(t, time.Duration(1), comps.Options.LeaderLeaseRefreshInterval)
	assert.Equal(t, 1, comps.Options.AggregationBuckets)
}

type wrongExtension struct{}

func (*wrongExtension) Start(context.Context, component.Host) error { return nil }
func (*wrongExtension) Shutdown(context.Context) error              { return nil }

func TestGetAdaptiveSamplingComponentsErrors(t *testing.T) {
	host := makeRemoteSamplingExtension(t, &Config{})
	_, err := GetAdaptiveSamplingComponents(host)
	require.ErrorContains(t, err, "extension 'remote_sampling' is not configured for adaptive sampling")

	h1 := storagetest.NewStorageHost()
	_, err = GetAdaptiveSamplingComponents(h1)
	require.ErrorContains(t, err, "cannot find extension")

	h2 := h1.WithExtension(ID, &wrongExtension{})
	_, err = GetAdaptiveSamplingComponents(h2)
	require.ErrorContains(t, err, "is not of type")
}

func TestDependencies(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)
	assert.Equal(t, []component.ID{jaegerstorage.ID}, ext.(*rsExtension).Dependencies())
}
