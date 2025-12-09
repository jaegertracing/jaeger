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
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/internal/config/promcfg"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
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

				ext := makeStorageExtension(t, storageconfig.Config{
					TraceBackends: map[string]storageconfig.TraceBackend{
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
				ext := makeStorageExtension(t, storageconfig.Config{
					TraceBackends: map[string]storageconfig.TraceBackend{
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
	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
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
	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
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
		config storageconfig.Config
	}{
		{
			name: "Prometheus",
			config: storageconfig.Config{
				MetricBackends: map[string]storageconfig.MetricBackend{
					"foo": {
						Prometheus: &storageconfig.PrometheusConfiguration{
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
			config: storageconfig.Config{
				MetricBackends: map[string]storageconfig.MetricBackend{
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
			config: storageconfig.Config{
				MetricBackends: map[string]storageconfig.MetricBackend{
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
	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
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
		config        storageconfig.Config
		expectedError string
	}{
		{
			name: "Prometheus backend initialization error",
			config: storageconfig.Config{
				MetricBackends: map[string]storageconfig.MetricBackend{
					"foo": {
						Prometheus: &storageconfig.PrometheusConfiguration{
							Configuration: promcfg.Configuration{},
						},
					},
				},
			},
			expectedError: "failed to initialize metrics storage 'foo'",
		},
		{
			name: "Elasticsearch backend initialization error",
			config: storageconfig.Config{
				MetricBackends: map[string]storageconfig.MetricBackend{
					"foo": {
						Elasticsearch: &escfg.Configuration{},
					},
				},
			},
			expectedError: "Servers: non zero value required",
		},
		{
			name: "OpenSearch backend initialization error",
			config: storageconfig.Config{
				MetricBackends: map[string]storageconfig.MetricBackend{
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

func TestElasticsearch(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
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
}

func TestOpenSearch(t *testing.T) {
	server := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
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
}

func TestCassandraError(t *testing.T) {
	// since we cannot successfully create storage factory for Cassandra
	// without running a Cassandra server, we only test the error case.
	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
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
	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
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

func makeStorageExtension(t *testing.T, config storageconfig.Config) component.Component {
	extensionFactory := NewFactory()
	ctx := t.Context()
	ext, err := extensionFactory.Create(ctx,
		extension.Settings{
			ID:                ID,
			TelemetrySettings: noopTelemetrySettings(),
			BuildInfo:         component.NewDefaultBuildInfo(),
		},
		&Config{Config: config},
	)
	require.NoError(t, err)
	return ext
}

func TestStorageBackend_DefaultCases(t *testing.T) {
	config := storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"unconfigured": {},
		},
	}

	ext := makeStorageExtension(t, config)
	err := ext.Start(t.Context(), componenttest.NewNopHost())

	require.Error(t, err)
	require.Contains(t, err.Error(), "empty configuration")

	config = storageconfig.Config{
		MetricBackends: map[string]storageconfig.MetricBackend{
			"unconfigured": {},
		},
	}

	ext = makeStorageExtension(t, config)
	err = ext.Start(t.Context(), componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no metric backend configuration provided")
}

func startStorageExtension(t *testing.T, memstoreName string, promstoreName string) component.Component {
	config := storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			memstoreName: {
				Memory: &memory.Configuration{
					MaxTraces: 10000,
				},
			},
		},
		MetricBackends: map[string]storageconfig.MetricBackend{
			promstoreName: {
				Prometheus: &storageconfig.PrometheusConfiguration{
					Configuration: promcfg.Configuration{
						ServerURL: "localhost:12345",
					},
				},
			},
		},
	}
	require.NoError(t, (&Config{Config: config}).Validate())

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
		WithExtension(ID, makeStorageExtension(t, storageconfig.Config{
			MetricBackends: map[string]storageconfig.MetricBackend{
				"prometheus": {
					Prometheus: &storageconfig.PrometheusConfiguration{
						Configuration: promcfg.Configuration{
							ServerURL: mockServer.URL,
						},
						Authentication: escfg.Authentication{
							Config: configauth.Config{
								AuthenticatorID: component.MustNewID("sigv4auth"),
							},
						},
					},
				},
			},
		})).
		WithExtension(component.MustNewID("sigv4auth"), mockAuth)

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

	config := storageconfig.Config{
		MetricBackends: map[string]storageconfig.MetricBackend{
			"prometheus": {
				Prometheus: &storageconfig.PrometheusConfiguration{
					Configuration: promcfg.Configuration{
						ServerURL: mockServer.URL,
					},
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("sigv4auth"),
						},
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
	const (
		backendType = "elasticsearch"
		backendName = "test"
	)

	tests := []struct {
		name        string
		authCfg     escfg.Authentication
		setupHost   func() component.Host
		wantErr     bool
		errContains string
	}{
		{
			name:      "empty authenticator returns nil",
			authCfg:   escfg.Authentication{},
			setupHost: componenttest.NewNopHost,
			wantErr:   false,
		},
		{
			name: "valid authenticator",
			authCfg: escfg.Authentication{
				Config: configauth.Config{
					AuthenticatorID: component.MustNewID("sigv4auth"),
				},
			},
			setupHost: func() component.Host {
				return storagetest.NewStorageHost().
					WithExtension(component.MustNewIDWithName("sigv4auth", "sigv4auth"), &mockHTTPAuthenticator{})
			},
			wantErr: false,
		},
		{
			name: "authenticator not found",
			authCfg: escfg.Authentication{
				Config: configauth.Config{
					AuthenticatorID: component.MustNewID("notfound"),
				},
			},
			setupHost:   componenttest.NewNopHost,
			wantErr:     true,
			errContains: "failed to get HTTP authenticator for elasticsearch backend 'test'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := &storageExt{telset: noopTelemetrySettings()}
			host := tt.setupHost()

			auth, err := ext.resolveAuthenticator(host, tt.authCfg, backendType, backendName)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			// Check if authenticator ID is empty
			if tt.authCfg.AuthenticatorID.String() == "" {
				require.Nil(t, auth)
			} else {
				require.NotNil(t, auth)
			}
		})
	}
}

// Test getAuthenticator with empty authenticator name
func TestGetAuthenticatorEmptyName(t *testing.T) {
	cfg := &Config{}
	ext := newStorageExt(cfg, noopTelemetrySettings())

	host := componenttest.NewNopHost()

	// Call with empty authenticator name
	auth, err := ext.getAuthenticator(host, "")

	require.NoError(t, err)
	require.Nil(t, auth)
}

// Test Elasticsearch with valid authenticator integration
func TestElasticsearchWithAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	mockAuth := &mockHTTPAuthenticator{}

	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"elasticsearch": {
				Elasticsearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("sigv4auth"),
						},
					},
				},
			},
		},
	})
	host := storagetest.NewStorageHost().
		WithExtension(ID, ext).
		WithExtension(component.MustNewID("sigv4auth"), mockAuth)

	err := ext.Start(t.Context(), host)
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(t.Context()))
}

// Test OpenSearch with valid authenticator integration
func TestOpenSearchWithAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	mockAuth := &mockHTTPAuthenticator{}

	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"opensearch": {
				Opensearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("sigv4auth"),
						},
					},
				},
			},
		},
	})
	host := storagetest.NewStorageHost().
		WithExtension(ID, ext).
		WithExtension(component.MustNewID("sigv4auth"), mockAuth)

	err := ext.Start(t.Context(), host)
	require.NoError(t, err)
	require.NoError(t, ext.Shutdown(t.Context()))
}

// Test Elasticsearch with missing authenticator
func TestElasticsearchWithMissingAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)

	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"elasticsearch": {
				Elasticsearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("nonexistent"),
						},
					},
				},
			},
		},
	})
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get HTTP authenticator")
}

// Test OpenSearch trace backend with missing authenticator
func TestOpenSearchTraceWithMissingAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)

	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"opensearch": {
				Opensearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("nonexistent"),
						},
					},
				},
			},
		},
	})
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get HTTP authenticator")
}

// Test Elasticsearch with wrong authenticator type
func TestElasticsearchWithWrongAuthenticatorType(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	wrongAuth := &mockNonHTTPExtension{}

	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"elasticsearch": {
				Elasticsearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("wrongtype"),
						},
					},
				},
			},
		},
	})
	host := storagetest.NewStorageHost().
		WithExtension(ID, ext).
		WithExtension(component.MustNewID("wrongtype"), wrongAuth)

	err := ext.Start(t.Context(), host)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not implement extensionauth.HTTPClient")
}

// Test OpenSearch with wrong authenticator type
func TestOpenSearchWithWrongAuthenticatorType(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)
	wrongAuth := &mockNonHTTPExtension{}

	ext := makeStorageExtension(t, storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"opensearch": {
				Opensearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("wrongtype"),
						},
					},
				},
			},
		},
	})
	host := storagetest.NewStorageHost().
		WithExtension(ID, ext).
		WithExtension(component.MustNewID("wrongtype"), wrongAuth)

	err := ext.Start(t.Context(), host)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not implement extensionauth.HTTPClient")
}

// Test Elasticsearch metrics backend with invalid authenticator
func TestElasticsearchMetricsWithInvalidAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)

	ext := makeStorageExtension(t, storageconfig.Config{
		MetricBackends: map[string]storageconfig.MetricBackend{
			"elasticsearch": {
				Elasticsearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("nonexistent"),
						},
					},
				},
			},
		},
	})
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get HTTP authenticator")
}

// Test OpenSearch metrics backend with invalid authenticator
func TestOpenSearchMetricsWithInvalidAuthenticator(t *testing.T) {
	mockServer := setupMockServer(t, getVersionResponse(t), http.StatusOK)

	ext := makeStorageExtension(t, storageconfig.Config{
		MetricBackends: map[string]storageconfig.MetricBackend{
			"opensearch": {
				Opensearch: &escfg.Configuration{
					Servers:  []string{mockServer.URL},
					LogLevel: "error",
					Authentication: escfg.Authentication{
						Config: configauth.Config{
							AuthenticatorID: component.MustNewID("nonexistent"),
						},
					},
				},
			},
		},
	})
	err := ext.Start(t.Context(), componenttest.NewNopHost())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get HTTP authenticator")
}
