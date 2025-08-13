// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync"
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

// TestStartFileBasedProviderError tests error handling in file-based provider startup
func TestStartFileBasedProviderError(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)

	// Configure with invalid file path to trigger error
	cfg.File = &FileConfig{
		Path:                       "/nonexistent/directory/file.json",
		ReloadInterval:             0,
		DefaultSamplingProbability: 0.1,
	}
	cfg.Adaptive = nil
	cfg.HTTP = nil
	cfg.GRPC = nil

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)

	// This should trigger the error path in startFileBasedStrategyProvider
	err = ext.Start(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create the local file strategy store")
}

// TestStartAdaptiveProviderCreateStoreError tests error handling in adaptive provider startup
func TestStartAdaptiveProviderCreateStoreError(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.File = nil
	cfg.Adaptive = &AdaptiveConfig{
		SamplingStore: "nonexistent-store",
		Options:       adaptive.DefaultOptions(),
	}
	cfg.HTTP = nil
	cfg.GRPC = nil

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)

	// This should trigger error in startAdaptiveStrategyProvider due to invalid storage
	err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to obtain sampling store factory")
}

// TestServerStartupErrors tests various server startup error scenarios
func TestServerStartupErrors(t *testing.T) {
	t.Run("HTTP server with invalid endpoint", func(t *testing.T) {
		factory := NewFactory()
		cfg := factory.CreateDefaultConfig().(*Config)
		cfg.File = &FileConfig{Path: filepath.Join("..", "..", "..", "sampling-strategies.json")}
		cfg.Adaptive = nil
		cfg.HTTP = &confighttp.ServerConfig{
			Endpoint: "invalid://endpoint", // Invalid URL scheme
		}
		cfg.GRPC = nil

		ext, err := factory.Create(context.Background(), extension.Settings{
			ID:                ID,
			TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		}, cfg)
		require.NoError(t, err)

		// This should trigger an error in Start() -> startHTTPServer()
		err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
		require.Error(t, err)

		// Ensure cleanup
		_ = ext.Shutdown(context.Background())
	})

	t.Run("gRPC server with invalid endpoint", func(t *testing.T) {
		factory := NewFactory()
		cfg := factory.CreateDefaultConfig().(*Config)
		cfg.File = &FileConfig{Path: filepath.Join("..", "..", "..", "sampling-strategies.json")}
		cfg.Adaptive = nil
		cfg.HTTP = nil
		cfg.GRPC = &configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "invalid://endpoint", // Invalid URL scheme
				Transport: "tcp",
			},
		}

		ext, err := factory.Create(context.Background(), extension.Settings{
			ID:                ID,
			TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		}, cfg)
		require.NoError(t, err)

		// This should trigger an error in Start() -> startGRPCServer()
		err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
		require.Error(t, err)

		// Ensure cleanup
		_ = ext.Shutdown(context.Background())
	})
}

// TestShutdownWithProviderError tests shutdown when strategy provider fails to close
func TestShutdownWithProviderError(t *testing.T) {
	t.Run("error from strategy provider", func(t *testing.T) {
		ext := &rsExtension{
			cfg:       &Config{},
			telemetry: componenttest.NewNopTelemetrySettings(),
		}

		// Mock a strategy provider that will fail on Close()
		ext.strategyProvider = &mockFailingProvider{}

		err := ext.Shutdown(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mock provider close error")
	})

	t.Run("error from HTTP server shutdown", func(t *testing.T) {
		ext := &rsExtension{
			cfg:       &Config{},
			telemetry: componenttest.NewNopTelemetrySettings(),
		}

		// Create a server that will have active connections during shutdown
		srv := &http.Server{
			Addr: ":0", // use dynamic port
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				// Simulate a slow request that will still be running during shutdown
				time.Sleep(100 * time.Millisecond)
				w.Write([]byte("done"))
			}),
		}

		ln, err := net.Listen("tcp", srv.Addr)
		require.NoError(t, err)
		defer ln.Close()

		// Set the server in the extension
		ext.httpServer = srv

		// Start the server
		go srv.Serve(ln)

		var wg sync.WaitGroup
		wg.Add(1)

		// Fire off a request that will still be running during shutdown
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 200 * time.Millisecond}
			_, _ = client.Get("http://" + ln.Addr().String())
		}()

		// Give the handler time to start processing the request
		time.Sleep(10 * time.Millisecond)

		// Use a very short timeout context so shutdown fails due to active connections
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()

		// This should trigger the error path in the extension's Shutdown method
		err = ext.Shutdown(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")

		// Clean up: ensure the request completes and server shuts down properly
		wg.Wait()
		_ = srv.Shutdown(context.Background())
	})
}

// Mock types for testing shutdown error paths

// mockFailingProvider is a mock that always fails on Close()
type mockFailingProvider struct{}

func (*mockFailingProvider) GetSamplingStrategy(ctx context.Context, serviceName string) (*api_v2.SamplingStrategyResponse, error) {
	return &api_v2.SamplingStrategyResponse{}, nil
}

func (*mockFailingProvider) Close() error {
	return errors.New("mock provider close error")
}
