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
	"go.opentelemetry.io/collector/config/configmiddleware"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	samplingstoremodel "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
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
		&jaegerstorage.Config{
			Config: storageconfig.Config{
				TraceBackends: map[string]storageconfig.TraceBackend{
					memstoreName: {Memory: &memory.Configuration{MaxTraces: 10000}},
				},
			},
		},
	)
	require.NoError(t, err)

	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, storageExtension)

	err = storageExtension.Start(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storageExtension.Shutdown(context.Background())) })
	return host
}

func makeStorageExtensionWithBadSamplingStore(storageName string) component.Host {
	ext := &fakeStorageExtensionForTest{
		storageName: storageName,
		failOn:      "CreateSamplingStore",
	}
	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, ext)
	return host
}

func makeStorageExtensionWithBadLock(storageName string) component.Host {
	ext := &fakeStorageExtensionForTest{
		storageName: storageName,
		failOn:      "CreateLock",
	}
	host := storagetest.NewStorageHost()
	host.WithExtension(jaegerstorage.ID, ext)
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
	cfg.File = configoptional.Some(FileConfig{
		Path: filepath.Join("..", "..", "..", "sampling-strategies.json"),
	})
	cfg.Adaptive = configoptional.None[AdaptiveConfig]()
	cfg.HTTP = configoptional.None[confighttp.ServerConfig]()
	cfg.GRPC = configoptional.None[configgrpc.ServerConfig]()
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
	cfg.File = configoptional.Some(FileConfig{
		Path: filepath.Join("..", "..", "..", "sampling-strategies.json"),
	})
	cfg.Adaptive = configoptional.None[AdaptiveConfig]()
	cfg.HTTP = configoptional.Some(confighttp.ServerConfig{
		NetAddr: confignet.AddrConfig{
			Endpoint:  "0.0.0.0:12345",
			Transport: confignet.TransportTypeTCP,
		},
	})
	cfg.GRPC = configoptional.None[configgrpc.ServerConfig]()
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

	// The expected response uses ProtoJSON encoding of enums (strings, not numbers).
	// Cf. https://github.com/jaegertracing/jaeger/pull/8014
	expectedResponse := `{
        "probabilisticSampling": {
            "samplingRate": 0.8
        },
        "strategyType": "PROBABILISTIC"
    }`
	require.JSONEq(t, expectedResponse, string(body))

	require.NoError(t, ext.Shutdown(context.Background()))
}

func TestStartGRPC(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.File = configoptional.Some(FileConfig{
		Path: filepath.Join("..", "..", "..", "sampling-strategies.json"),
	})
	cfg.Adaptive = configoptional.None[AdaptiveConfig]()
	cfg.HTTP = configoptional.None[confighttp.ServerConfig]()
	cfg.GRPC = configoptional.Some(configgrpc.ServerConfig{
		NetAddr: confignet.AddrConfig{
			Endpoint:  "0.0.0.0:12346",
			Transport: "tcp",
		},
	})
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
	cfg.File = configoptional.None[FileConfig]()
	cfg.Adaptive = configoptional.Some(AdaptiveConfig{
		SamplingStore: "foobar",
		Options:       adaptive.DefaultOptions(),
	})
	cfg.HTTP = configoptional.None[confighttp.ServerConfig]()
	cfg.GRPC = configoptional.None[configgrpc.ServerConfig]()
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
			Adaptive: configoptional.Some(AdaptiveConfig{
				SamplingStore: "foobar",
			}),
		},
	}
	err := ext.startAdaptiveStrategyProvider(host)
	require.ErrorContains(t, err, "failed to obtain sampling store factory")
}

func TestStartAdaptiveStrategyProviderCreateStoreError(t *testing.T) {
	// storage extension has the requested store name but its factory fails on CreateSamplingStore
	storageHost := makeStorageExtensionWithBadSamplingStore("failstore")

	ext := &rsExtension{
		cfg: &Config{
			Adaptive: configoptional.Some(AdaptiveConfig{
				SamplingStore: "failstore",
				Options: adaptive.Options{
					AggregationBuckets: 10,
				},
			}),
		},
		telemetry: componenttest.NewNopTelemetrySettings(),
	}
	err := ext.startAdaptiveStrategyProvider(storageHost)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create the sampling store")
}

func TestStartAdaptiveStrategyProviderCreateLockError(t *testing.T) {
	storageHost := makeStorageExtensionWithBadLock("lockerror")

	ext := &rsExtension{
		cfg: &Config{
			Adaptive: configoptional.Some(AdaptiveConfig{
				SamplingStore: "lockerror",
				Options: adaptive.Options{
					AggregationBuckets: 10,
				},
			}),
		},
		telemetry: componenttest.NewNopTelemetrySettings(),
	}
	err := ext.startAdaptiveStrategyProvider(storageHost)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create the distributed lock")
}

func TestGetAdaptiveSamplingComponents(t *testing.T) {
	// Success case
	host := makeRemoteSamplingExtension(t, &Config{
		Adaptive: configoptional.Some(AdaptiveConfig{
			SamplingStore: "foobar",
			Options: adaptive.Options{
				FollowerLeaseRefreshInterval: 1,
				LeaderLeaseRefreshInterval:   1,
				AggregationBuckets:           1,
			},
		}),
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

type fakeStorageExtensionForTest struct {
	storageName string
	failOn      string
}

func (*fakeStorageExtensionForTest) Start(context.Context, component.Host) error { return nil }
func (*fakeStorageExtensionForTest) Shutdown(context.Context) error              { return nil }

func (f *fakeStorageExtensionForTest) TraceStorageFactory(name string) (tracestore.Factory, error) {
	if name == f.storageName {
		return &fakeSamplingStoreFactory{failOn: f.failOn}, nil
	}
	return nil, errors.New("storage not found")
}

func (*fakeStorageExtensionForTest) MetricStorageFactory(string) (storage.MetricStoreFactory, error) {
	return nil, errors.New("metric storage not found")
}

type fakeSamplingStoreFactory struct {
	failOn string
}

func (*fakeSamplingStoreFactory) CreateTraceReader() (tracestore.Reader, error) {
	return &tracestoremocks.Reader{}, nil
}

func (*fakeSamplingStoreFactory) CreateTraceWriter() (tracestore.Writer, error) {
	return &tracestoremocks.Writer{}, nil
}

func (f *fakeSamplingStoreFactory) CreateSamplingStore(int) (samplingstore.Store, error) {
	if f.failOn == "CreateSamplingStore" {
		return nil, errors.New("mock error creating sampling store")
	}
	return &samplingStoreMock{}, nil
}

func (f *fakeSamplingStoreFactory) CreateLock() (distributedlock.Lock, error) {
	if f.failOn == "CreateLock" {
		return nil, errors.New("mock error creating lock")
	}
	return &lockMock{}, nil
}

type samplingStoreMock struct{}

func (*samplingStoreMock) GetThroughput(time.Time, time.Time) ([]*samplingstoremodel.Throughput, error) {
	return nil, nil
}

func (*samplingStoreMock) GetLatestProbabilities() (samplingstoremodel.ServiceOperationProbabilities, error) {
	return nil, nil
}

func (*samplingStoreMock) InsertThroughput([]*samplingstoremodel.Throughput) error {
	return nil
}

func (*samplingStoreMock) InsertProbabilitiesAndQPS(string, samplingstoremodel.ServiceOperationProbabilities, samplingstoremodel.ServiceOperationQPS) error {
	return nil
}

type lockMock struct{}

func (*lockMock) Acquire(string, time.Duration) (bool, error) {
	return true, nil
}

func (*lockMock) Forfeit(string) (bool, error) {
	return true, nil
}

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

	cfg.File = configoptional.Some(FileConfig{
		Path:                       "/nonexistent/directory/file.json",
		ReloadInterval:             0,
		DefaultSamplingProbability: 0.1,
	})
	cfg.Adaptive = configoptional.None[AdaptiveConfig]()
	cfg.HTTP = configoptional.None[confighttp.ServerConfig]()
	cfg.GRPC = configoptional.None[configgrpc.ServerConfig]()

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)

	err = ext.Start(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create the local file strategy store")
}

// TestStartAdaptiveProviderCreateStoreError tests error handling in adaptive provider startup
func TestStartAdaptiveProviderCreateStoreError(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.File = configoptional.None[FileConfig]()
	cfg.Adaptive = configoptional.Some(AdaptiveConfig{
		SamplingStore: "nonexistent-store",
		Options:       adaptive.DefaultOptions(),
	})
	cfg.HTTP = configoptional.None[confighttp.ServerConfig]()
	cfg.GRPC = configoptional.None[configgrpc.ServerConfig]()

	ext, err := factory.Create(context.Background(), extension.Settings{
		ID:                ID,
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg)
	require.NoError(t, err)

	err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to obtain sampling store factory")
}

// TestServerStartupErrors tests various server startup error scenarios
func TestServerStartupErrors(t *testing.T) {
	t.Run("HTTP server with invalid endpoint", func(t *testing.T) {
		factory := NewFactory()
		cfg := factory.CreateDefaultConfig().(*Config)
		cfg.File = configoptional.Some(FileConfig{Path: filepath.Join("..", "..", "..", "sampling-strategies.json")})
		cfg.Adaptive = configoptional.None[AdaptiveConfig]()
		cfg.HTTP = configoptional.Some(confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "invalid://endpoint",
				Transport: confignet.TransportTypeTCP,
			},
		})
		cfg.GRPC = configoptional.None[configgrpc.ServerConfig]()

		ext, err := factory.Create(context.Background(), extension.Settings{
			ID:                ID,
			TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		}, cfg)
		require.NoError(t, err)

		err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
		require.Error(t, err)

		_ = ext.Shutdown(context.Background())
	})

	t.Run("gRPC server with invalid endpoint", func(t *testing.T) {
		factory := NewFactory()
		cfg := factory.CreateDefaultConfig().(*Config)
		cfg.File = configoptional.Some(FileConfig{Path: filepath.Join("..", "..", "..", "sampling-strategies.json")})
		cfg.Adaptive = configoptional.None[AdaptiveConfig]()
		cfg.HTTP = configoptional.None[confighttp.ServerConfig]()
		cfg.GRPC = configoptional.Some(configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "invalid://endpoint",
				Transport: "tcp",
			},
		})

		ext, err := factory.Create(context.Background(), extension.Settings{
			ID:                ID,
			TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		}, cfg)
		require.NoError(t, err)

		// This should trigger an error in Start() -> startGRPCServer()
		err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
		require.Error(t, err)

		_ = ext.Shutdown(context.Background())
	})

	t.Run("HTTP middleware not found error", func(t *testing.T) {
		factory := NewFactory()
		cfg := factory.CreateDefaultConfig().(*Config)
		cfg.File = configoptional.Some(FileConfig{Path: filepath.Join("..", "..", "..", "sampling-strategies.json")})
		cfg.Adaptive = configoptional.None[AdaptiveConfig]()
		cfg.HTTP = configoptional.Some(confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "localhost:0",
				Transport: confignet.TransportTypeTCP,
			},
			Middlewares: []configmiddleware.Config{
				{ID: component.MustNewIDWithName("nonexistent", "middleware")},
			},
		})
		cfg.GRPC = configoptional.None[configgrpc.ServerConfig]()

		ext, err := factory.Create(context.Background(), extension.Settings{
			ID:                ID,
			TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		}, cfg)
		require.NoError(t, err)

		// This should trigger the specific error from configmiddleware.go line 64-65:
		// return nil, fmt.Errorf("failed to resolve middleware %q: %w", m.ID, errMiddlewareNotFound)
		err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
		require.Error(t, err)

		require.Contains(t, err.Error(), "failed to start sampling http server")
		require.Contains(t, err.Error(), "failed to resolve middleware")
		require.Contains(t, err.Error(), "nonexistent/middleware")
		require.Contains(t, err.Error(), "middleware not found")

		_ = ext.Shutdown(context.Background())
	})

	t.Run("gRPC middleware not found error", func(t *testing.T) {
		factory := NewFactory()
		cfg := factory.CreateDefaultConfig().(*Config)
		cfg.File = configoptional.Some(FileConfig{Path: filepath.Join("..", "..", "..", "sampling-strategies.json")})
		cfg.Adaptive = configoptional.None[AdaptiveConfig]()
		cfg.HTTP = configoptional.None[confighttp.ServerConfig]()
		cfg.GRPC = configoptional.Some(configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "localhost:0",
				Transport: "tcp",
			},
			Middlewares: []configmiddleware.Config{
				{ID: component.MustNewIDWithName("nonexistent", "grpc-middleware")},
			},
		})

		ext, err := factory.Create(context.Background(), extension.Settings{
			ID:                ID,
			TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		}, cfg)
		require.NoError(t, err)

		// This should trigger the specific error from configmiddleware.go line 83-84:
		// return nil, fmt.Errorf("failed to resolve middleware %q: %w", m.ID, errMiddlewareNotFound)
		// called via gRPC ToServer() -> getGrpcServerOptions() -> GetGRPCServerOptions()
		err = ext.Start(context.Background(), makeStorageExtension(t, "foobar"))
		require.Error(t, err)

		require.Contains(t, err.Error(), "failed to start sampling gRPC server")
		require.Contains(t, err.Error(), "failed to get gRPC server options from middleware")
		require.Contains(t, err.Error(), "failed to resolve middleware")
		require.Contains(t, err.Error(), "nonexistent/grpc-middleware")
		require.Contains(t, err.Error(), "middleware not found")

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

		ext.httpServer = srv

		go srv.Serve(ln)

		var wg sync.WaitGroup

		// Fire off a request that will still be running during shutdown
		wg.Go(func() {
			client := &http.Client{Timeout: 200 * time.Millisecond}
			_, _ = client.Get("http://" + ln.Addr().String())
		})

		// Give the handler time to start processing the request
		time.Sleep(10 * time.Millisecond)

		// Use a very short timeout context so shutdown fails due to active connections
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()

		err = ext.Shutdown(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")

		wg.Wait()
		_ = srv.Shutdown(context.Background())
	})
}

// mockFailingProvider is a mock that always fails on Close()
type mockFailingProvider struct{}

func (*mockFailingProvider) GetSamplingStrategy(_ context.Context, _ string) (*api_v2.SamplingStrategyResponse, error) {
	return &api_v2.SamplingStrategyResponse{}, nil
}

func (*mockFailingProvider) Close() error {
	return errors.New("mock provider close error")
}
