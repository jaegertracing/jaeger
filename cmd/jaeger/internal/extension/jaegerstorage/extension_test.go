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
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/config/promcfg"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
)

type errorFactory struct {
	closeErr error
}

func (errorFactory) CreateTraceReader() (tracestore.Reader, error) {
	panic("not implemented")
}

func (errorFactory) CreateTraceWriter() (tracestore.Writer, error) {
	panic("not implemented")
}

func (errorFactory) CreateMetricsReader() (metricstore.Reader, error) { panic("not implemented") }

func (e errorFactory) Close() error {
	return e.closeErr
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

func TestStorageFactoryBadHostError(t *testing.T) {
	_, err := getStorageFactory("something", componenttest.NewNopHost())
	require.ErrorContains(t, err, "cannot find extension")
}

func TestStorageFactoryBadNameError(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(ID, startStorageExtension(t, "foo", ""))
	_, err := getStorageFactory("bar", host)
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
		factories: map[string]tracestore.Factory{
			"foo": errorFactory{closeErr: shutdownError},
		},
	}
	err := ext.Shutdown(t.Context())
	require.ErrorIs(t, err, shutdownError)
}

func TestGetFactoryV2Error(t *testing.T) {
	host := componenttest.NewNopHost()
	_, err := GetTraceStoreFactory("something", host)
	require.ErrorContains(t, err, "cannot find extension")
}

func TestGetFactory(t *testing.T) {
	const name = "foo"
	const metricname = "bar"
	host := storagetest.NewStorageHost().WithExtension(ID, startStorageExtension(t, name, metricname))
	f, err := getStorageFactory(name, host)
	require.NoError(t, err)
	require.NotNil(t, f)

	f2, err := GetTraceStoreFactory(name, host)
	require.NoError(t, err)
	require.NotNil(t, f2)

	f3, err := GetMetricStorageFactory(metricname, host)
	require.NoError(t, err)
	require.NotNil(t, f3)
}

func TestGetSamplingStoreFactory(t *testing.T) {
	tests := []struct {
		name          string
		storageName   string
		expectedError string
		setupFunc     func(t *testing.T) component.Component
	}{
		{
			name:        "Supported",
			storageName: "foo",
			setupFunc: func(t *testing.T) component.Component {
				traceStoreFactory := "foo"
				return startStorageExtension(t, traceStoreFactory, "bar")
			},
		},
		{
			name:          "NotFound",
			storageName:   "nonexistingstorage",
			expectedError: "cannot find definition of storage",
			setupFunc: func(t *testing.T) component.Component {
				traceStoreFactory := "foo"
				return startStorageExtension(t, traceStoreFactory, "bar")
			},
		},
		{
			name:          "NotSupported",
			storageName:   "foo",
			expectedError: "storage 'foo' does not support sampling store",
			setupFunc: func(t *testing.T) component.Component {
				versionResponse, err := json.Marshal(map[string]any{
					"Version": map[string]any{
						"Number": "7",
					},
				})
				require.NoError(t, err)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Write(versionResponse)
				}))
				t.Cleanup(func() { server.Close() })

				ext := makeStorageExtension(t, &Config{
					TraceBackends: map[string]TraceBackend{
						"foo": {
							Elasticsearch: &escfg.Configuration{
								Servers:  []string{server.URL},
								LogLevel: "error",
							},
						},
					},
				})
				require.NoError(t, ext.Start(t.Context(), componenttest.NewNopHost()))
				t.Cleanup(func() {
					require.NoError(t, ext.Shutdown(context.Background()))
				})
				return ext
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ext := test.setupFunc(t)
			host := storagetest.NewStorageHost().WithExtension(ID, ext)

			ssf, err := GetSamplingStoreFactory(test.storageName, host)
			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
				require.Nil(t, ssf)
			} else {
				require.NotNil(t, ssf)
			}
		})
	}
}

func TestGetPurger(t *testing.T) {
	tests := []struct {
		name          string
		storageName   string
		expectedError string
		setupFunc     func(t *testing.T) component.Component
	}{
		{
			name:        "Supported",
			storageName: "foo",
			setupFunc: func(t *testing.T) component.Component {
				traceStoreFactory := "foo"
				return startStorageExtension(t, traceStoreFactory, "bar")
			},
		},
		{
			name:          "NotFound",
			storageName:   "nonexistingstorage",
			expectedError: "cannot find definition of storage",
			setupFunc: func(t *testing.T) component.Component {
				traceStoreFactory := "foo"
				return startStorageExtension(t, traceStoreFactory, "bar")
			},
		},
		{
			name:          "NotSupported",
			storageName:   "foo",
			expectedError: "storage 'foo' does not support purging",
			setupFunc: func(t *testing.T) component.Component {
				ext := makeStorageExtension(t, &Config{
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
				require.NoError(t, ext.Start(t.Context(), componenttest.NewNopHost()))
				t.Cleanup(func() {
					require.NoError(t, ext.Shutdown(context.Background()))
				})
				return ext
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ext := test.setupFunc(t)
			host := storagetest.NewStorageHost().WithExtension(ID, ext)

			purger, err := GetPurger(test.storageName, host)
			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
				require.Nil(t, purger)
			} else {
				require.NotNil(t, purger)
			}
		})
	}
}

func TestBadger(t *testing.T) {
	ext := makeStorageExtension(t, &Config{
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
	ctx := t.Context()
	err := ext.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(ctx))
}

func TestGRPC(t *testing.T) {
	ext := makeStorageExtension(t, &Config{
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
	ctx := t.Context()
	err := ext.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(ctx))
}

func TestMetricBackends(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "Prometheus",
			config: &Config{
				MetricBackends: map[string]MetricBackend{
					"foo": {
						Prometheus: &PrometheusConfiguration{
							Configuration: promcfg.Configuration{
								ServerURL: mockServer.URL,
							},
						},
					},
				},
			},
		},
		{
			name: "Elasticsearch",
			config: &Config{
				MetricBackends: map[string]MetricBackend{
					"foo": {
						Elasticsearch: &escfg.Configuration{
							Servers:  []string{mockServer.URL},
							LogLevel: "info",
						},
					},
				},
			},
		},
		{
			name: "OpenSearch",
			config: &Config{
				MetricBackends: map[string]MetricBackend{
					"foo": {
						Opensearch: &escfg.Configuration{
							Servers:  []string{mockServer.URL},
							LogLevel: "info",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := makeStorageExtension(t, tt.config)
			ctx := t.Context()
			err := ext.Start(ctx, componenttest.NewNopHost())
			require.NoError(t, err)
			require.NoError(t, ext.Shutdown(ctx))
		})
	}
}

func TestMetricsBackendCloseError(t *testing.T) {
	shutdownError := errors.New("shutdown error")
	ext := storageExt{
		metricsFactories: map[string]storage.MetricStoreFactory{
			"foo": errorFactory{closeErr: shutdownError},
		},
	}
	err := ext.Shutdown(t.Context())
	require.ErrorIs(t, err, shutdownError)
}

func TestStartError(t *testing.T) {
	ext := makeStorageExtension(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"foo": {},
		},
	})
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "failed to initialize storage 'foo'")
	require.ErrorContains(t, err, "empty configuration")
}

func TestMetricStorageStartError(t *testing.T) {
	tests := []struct {
		name          string
		config        *Config
		expectedError string
	}{
		{
			name: "Prometheus backend initialization error",
			config: &Config{
				MetricBackends: map[string]MetricBackend{
					"foo": {
						Prometheus: &PrometheusConfiguration{
							Configuration: promcfg.Configuration{},
						},
					},
				},
			},
			expectedError: "failed to initialize metrics storage 'foo'",
		},
		{
			name: "Elasticsearch backend initialization error",
			config: &Config{
				MetricBackends: map[string]MetricBackend{
					"foo": {
						Elasticsearch: &escfg.Configuration{},
					},
				},
			},
			expectedError: "Servers: non zero value required",
		},
		{
			name: "OpenSearch backend initialization error",
			config: &Config{
				MetricBackends: map[string]MetricBackend{
					"foo": {
						Opensearch: &escfg.Configuration{},
					},
				},
			},
			expectedError: "Servers: non zero value required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := makeStorageExtension(t, tt.config)
			err := ext.Start(t.Context(), componenttest.NewNopHost())
			require.ErrorContains(t, err, tt.expectedError)
		})
	}
}

func testElasticsearchOrOpensearch(t *testing.T, cfg TraceBackend) {
	ext := makeStorageExtension(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"foo": cfg,
		},
	})
	ctx := t.Context()
	err := ext.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(ctx))
}

func TestXYZsearch(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	t.Run("Elasticsearch", func(t *testing.T) {
		ext := makeStorageExtension(t, &Config{
			TraceBackends: map[string]TraceBackend{
				"foo": {
					Elasticsearch: &escfg.Configuration{
						Servers:  []string{server.URL},
						LogLevel: "error",
					},
				},
			},
		})
		ctx := t.Context()
		err := ext.Start(ctx, componenttest.NewNopHost())
		require.NoError(t, err)
		require.NoError(t, ext.Shutdown(ctx))
	})
	t.Run("OpenSearch", func(t *testing.T) {
		ext := makeStorageExtension(t, &Config{
			TraceBackends: map[string]TraceBackend{
				"foo": {
					Opensearch: &escfg.Configuration{
						Servers:  []string{server.URL},
						LogLevel: "error",
					},
				},
			},
		})
		ctx := t.Context()
		err := ext.Start(ctx, componenttest.NewNopHost())
		require.NoError(t, err)
		require.NoError(t, ext.Shutdown(ctx))
	})
}

func TestCassandraError(t *testing.T) {
	// since we cannot successfully create storage factory for Cassandra
	// without running a Cassandra server, we only test the error case.
	ext := makeStorageExtension(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"cassandra": {
				Cassandra: &cassandra.Options{},
			},
		},
	})
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.ErrorContains(t, err, "failed to initialize storage 'cassandra'")
	require.ErrorContains(t, err, "Servers: non zero value required")
}

func TestClickHouse(t *testing.T) {
	testServer := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	t.Cleanup(testServer.Close)
	ext := makeStorageExtension(t, &Config{
		TraceBackends: map[string]TraceBackend{
			"foo": {
				ClickHouse: &clickhouse.Configuration{
					Protocol: "http",
					Addresses: []string{
						testServer.Listener.Addr().String(),
					},
				},
			},
		},
	})
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(t.Context()))
}

func noopTelemetrySettings() component.TelemetrySettings {
	return component.TelemetrySettings{
		Logger:         zap.L(),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}
}

func makeStorageExtension(t *testing.T, config *Config) component.Component {
	extensionFactory := NewFactory()
	ctx := t.Context()
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

func TestStorageBackend_DefaultCases(t *testing.T) {
	config := &Config{
		TraceBackends: map[string]TraceBackend{
			"unconfigured": {},
		},
	}

	ext := makeStorageExtension(t, config)
	err := ext.Start(t.Context(), componenttest.NewNopHost())

	require.Error(t, err)
	require.Contains(t, err.Error(), "empty configuration")

	config = &Config{
		MetricBackends: map[string]MetricBackend{
			"unconfigured": {},
		},
	}

	ext = makeStorageExtension(t, config)
	err = ext.Start(t.Context(), componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no metric backend configuration provided")
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
				Prometheus: &PrometheusConfiguration{
					Configuration: promcfg.Configuration{
						ServerURL: "localhost:12345",
					},
				},
			},
		},
	}
	require.NoError(t, config.Validate())

	ext := makeStorageExtension(t, config)
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, ext.Shutdown(context.Background()))
	})
	return ext
}

// Test authenticator resolution - success case
func TestGetAuthenticator_Success(t *testing.T) {
	mockAuth := &mockHTTPAuthenticator{}

	host := storagetest.NewStorageHost().
		WithExtension(component.MustNewIDWithName("sigv4auth", "sigv4auth"), mockAuth)

	cfg := &Config{}
	ext := newStorageExt(cfg, noopTelemetrySettings())

	auth, err := ext.getAuthenticator(host, "sigv4auth")
	require.NoError(t, err)
	require.NotNil(t, auth)
}

// Test authenticator not found
func TestGetAuthenticator_NotFound(t *testing.T) {
	host := componenttest.NewNopHost()

	cfg := &Config{}
	ext := newStorageExt(cfg, noopTelemetrySettings())

	auth, err := ext.getAuthenticator(host, "nonexistent")
	require.Error(t, err)
	require.Nil(t, auth)
	require.Contains(t, err.Error(), "authenticator extension 'nonexistent' not found")
}

// Test authenticator wrong type
func TestGetAuthenticator_WrongType(t *testing.T) {
	mockExt := &mockNonHTTPExtension{}

	host := storagetest.NewStorageHost().
		WithExtension(component.MustNewIDWithName("wrongtype", "wrongtype"), mockExt)

	cfg := &Config{}
	ext := newStorageExt(cfg, noopTelemetrySettings())

	auth, err := ext.getAuthenticator(host, "wrongtype")
	require.Error(t, err)
	require.Nil(t, auth)
	require.Contains(t, err.Error(), "does not implement extensionauth.HTTPClient")
}

// Test metric backend with valid authenticator
func TestMetricBackendWithAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	mockAuth := &mockHTTPAuthenticator{}

	host := storagetest.NewStorageHost().
		WithExtension(ID, makeStorageExtension(t, &Config{
			MetricBackends: map[string]MetricBackend{
				"prometheus": {
					Prometheus: &PrometheusConfiguration{
						Configuration: promcfg.Configuration{
							ServerURL: mockServer.URL,
						},
						Auth: &AuthConfig{
							Authenticator: "sigv4auth",
						},
					},
				},
			},
		})).
		WithExtension(component.MustNewIDWithName("sigv4auth", "sigv4auth"), mockAuth)

	ext := host.GetExtensions()[ID]
	require.NoError(t, ext.Start(t.Context(), host))

	factory, err := GetMetricStorageFactory("prometheus", host)
	require.NoError(t, err)
	require.NotNil(t, factory)

	t.Cleanup(func() {
		require.NoError(t, ext.(extension.Extension).Shutdown(context.Background()))
	})
}

// Test metric backend with invalid authenticator name
func TestMetricBackendWithInvalidAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)

	config := &Config{
		MetricBackends: map[string]MetricBackend{
			"prometheus": {
				Prometheus: &PrometheusConfiguration{
					Configuration: promcfg.Configuration{
						ServerURL: mockServer.URL,
					},
					Auth: &AuthConfig{
						Authenticator: "nonexistent",
					},
				},
			},
		},
	}

	ext := makeStorageExtension(t, config)
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get HTTP authenticator")
}

// Mock HTTP authenticator for testing
type mockHTTPAuthenticator struct {
	component.Component
}

func (*mockHTTPAuthenticator) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	return &mockRoundTripper{base: base}, nil
}

func (*mockHTTPAuthenticator) Start(context.Context, component.Host) error {
	return nil
}

func (*mockHTTPAuthenticator) Shutdown(context.Context) error {
	return nil
}

// Mock RoundTripper for testing
type mockRoundTripper struct {
	base http.RoundTripper
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer mock-token")
	if m.base != nil {
		return m.base.RoundTrip(req)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// Mock non-HTTP extension for testing wrong type scenario
type mockNonHTTPExtension struct {
	component.Component
}

func (*mockNonHTTPExtension) Start(context.Context, component.Host) error {
	return nil
}

func (*mockNonHTTPExtension) Shutdown(context.Context) error {
	return nil
}

// Test resolveAuthenticator helper
func TestResolveAuthenticator(t *testing.T) {
	tests := []struct {
		name        string
		authCfg     *escfg.AuthExtensionConfig
		setupHost   func() component.Host
		backendType string
		backendName string
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config returns nil",
			authCfg:     nil,
			setupHost:   componenttest.NewNopHost,
			backendType: "elasticsearch",
			backendName: "test",
			wantErr:     false,
		},
		{
			name:        "empty authenticator returns nil",
			authCfg:     &escfg.AuthExtensionConfig{Authenticator: ""},
			setupHost:   componenttest.NewNopHost,
			backendType: "elasticsearch",
			backendName: "test",
			wantErr:     false,
		},
		{
			name:    "valid authenticator",
			authCfg: &escfg.AuthExtensionConfig{Authenticator: "sigv4auth"},
			setupHost: func() component.Host {
				return storagetest.NewStorageHost().
					WithExtension(component.MustNewIDWithName("sigv4auth", "sigv4auth"), &mockHTTPAuthenticator{})
			},
			backendType: "elasticsearch",
			backendName: "test",
			wantErr:     false,
		},
		{
			name:        "authenticator not found",
			authCfg:     &escfg.AuthExtensionConfig{Authenticator: "notfound"},
			setupHost:   componenttest.NewNopHost,
			backendType: "elasticsearch",
			backendName: "test",
			wantErr:     true,
			errContains: "failed to get HTTP authenticator for elasticsearch backend 'test'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := &storageExt{telset: noopTelemetrySettings()}
			host := tt.setupHost()

			auth, err := ext.resolveAuthenticator(host, tt.authCfg, tt.backendType, tt.backendName)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			if tt.authCfg == nil || tt.authCfg.Authenticator == "" {
				require.Nil(t, auth)
			} else {
				require.NotNil(t, auth)
			}
		})
	}
}
