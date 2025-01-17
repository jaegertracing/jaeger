// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confignet"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	v2querysvc "github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/internal/grpctest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/metricstore"
	metricstoremocks "github.com/jaegertracing/jaeger/storage/metricstore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

type fakeFactory struct {
	name string
}

func (ff fakeFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	if ff.name == "need-dependency-reader-error" {
		return nil, errors.New("test-error")
	}
	return &depsmocks.Reader{}, nil
}

func (ff fakeFactory) CreateSpanReader() (spanstore.Reader, error) {
	if ff.name == "need-span-reader-error" {
		return nil, errors.New("test-error")
	}
	return &spanstoremocks.Reader{}, nil
}

func (ff fakeFactory) CreateSpanWriter() (spanstore.Writer, error) {
	if ff.name == "need-span-writer-error" {
		return nil, errors.New("test-error")
	}
	return &spanstoremocks.Writer{}, nil
}

func (ff fakeFactory) Initialize(metrics.Factory, *zap.Logger) error {
	if ff.name == "need-initialize-error" {
		return errors.New("test-error")
	}
	return nil
}

type fakeMetricsFactory struct {
	name string
}

// Initialize implements storage.MetricsFactory.
func (fmf fakeMetricsFactory) Initialize(telemetry.Settings) error {
	if fmf.name == "need-initialize-error" {
		return errors.New("test-error")
	}
	return nil
}

func (fmf fakeMetricsFactory) CreateMetricsReader() (metricstore.Reader, error) {
	if fmf.name == "need-metrics-reader-error" {
		return nil, errors.New("test-error")
	}
	return &metricstoremocks.Reader{}, nil
}

type fakeStorageExt struct{}

var _ jaegerstorage.Extension = (*fakeStorageExt)(nil)

func (fakeStorageExt) TraceStorageFactory(name string) (storage.Factory, bool) {
	if name == "need-factory-error" {
		return nil, false
	}

	return fakeFactory{name: name}, true
}

func (fakeStorageExt) MetricStorageFactory(name string) (storage.MetricStoreFactory, bool) {
	if name == "need-factory-error" {
		return nil, false
	}
	return fakeMetricsFactory{name: name}, true
}

func (fakeStorageExt) Start(context.Context, component.Host) error {
	return nil
}

func (fakeStorageExt) Shutdown(context.Context) error {
	return nil
}

func TestServerDependencies(t *testing.T) {
	expectedDependencies := []component.ID{jaegerstorage.ID}
	telemetrySettings := component.TelemetrySettings{
		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
	}

	server := newServer(createDefaultConfig().(*Config), telemetrySettings)
	dependencies := server.Dependencies()

	assert.Equal(t, expectedDependencies, dependencies)
}

func TestServerStart(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(jaegerstorage.ID, fakeStorageExt{})
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "Real server with non-empty config",
			config: &Config{
				Storage: Storage{
					TracesArchive: "jaeger_storage",
					TracesPrimary: "jaeger_storage",
					Metrics:       "jaeger_metrics_storage",
				},
			},
		},
		{
			name: "factory error",
			config: &Config{
				Storage: Storage{
					TracesPrimary: "need-factory-error",
				},
			},
			expectedErr: "cannot find factory for trace storage",
		},
		{
			name: "span reader error",
			config: &Config{
				Storage: Storage{
					TracesPrimary: "need-span-reader-error",
				},
			},
			expectedErr: "cannot create trace reader",
		},
		{
			name: "dependency error",
			config: &Config{
				Storage: Storage{
					TracesPrimary: "need-dependency-reader-error",
				},
			},
			expectedErr: "cannot create dependencies reader",
		},
		{
			name: "storage archive error",
			config: &Config{
				Storage: Storage{
					TracesArchive: "need-factory-error",
					TracesPrimary: "jaeger_storage",
				},
			},
			expectedErr: "cannot find traces archive storage factory",
		},
		{
			name: "metrics storage error",
			config: &Config{
				Storage: Storage{
					Metrics:       "need-factory-error",
					TracesPrimary: "jaeger_storage",
				},
			},
			expectedErr: "cannot find metrics storage factory",
		},
		{
			name: " metrics reader error",
			config: &Config{
				Storage: Storage{
					Metrics:       "need-metrics-reader-error",
					TracesPrimary: "jaeger_storage",
				},
			},
			expectedErr: "cannot create metrics reader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Despite using Noop Tracer below, query service also creates jtracer.
			// We want to prevent that tracer from sampling anything in this test.
			t.Setenv("OTEL_TRACES_SAMPLER", "always_off")
			telemetrySettings := component.TelemetrySettings{
				Logger:         zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
				MeterProvider:  noopmetric.NewMeterProvider(),
				TracerProvider: nooptrace.NewTracerProvider(),
			}
			tt.config.HTTP.Endpoint = "localhost:0"
			tt.config.GRPC.NetAddr.Endpoint = "localhost:0"
			tt.config.GRPC.NetAddr.Transport = confignet.TransportTypeTCP
			server := newServer(tt.config, telemetrySettings)
			err := server.Start(context.Background(), host)
			if tt.expectedErr == "" {
				require.NoError(t, err)
				defer server.Shutdown(context.Background())
				// We need to wait for servers to become available.
				// Otherwise, we could call shutdown before the servers are even started,
				// which could cause flaky code coverage by going through error cases.
				require.Eventually(t,
					func() bool {
						resp, err := http.Get(fmt.Sprintf("http://%s/", server.server.HTTPAddr()))
						if err != nil {
							return false
						}
						defer resp.Body.Close()
						return resp.StatusCode == http.StatusOK
					},
					10*time.Second,
					100*time.Millisecond,
					"server not started")
				grpctest.ReflectionServiceValidator{
					HostPort: server.server.GRPCAddr(),
					ExpectedServices: []string{
						"jaeger.api_v2.QueryService",
						"jaeger.api_v3.QueryService",
						"jaeger.api_v2.metrics.MetricsQueryService",
						"grpc.health.v1.Health",
					},
				}.Execute(t)
			} else {
				require.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}

func TestServerAddArchiveStorage(t *testing.T) {
	host := componenttest.NewNopHost()

	tests := []struct {
		name           string
		qSvcOpts       *querysvc.QueryServiceOptions
		v2qSvcOpts     *v2querysvc.QueryServiceOptions
		config         *Config
		extension      component.Component
		expectedOutput string
		expectedErr    string
	}{
		{
			name:           "Archive storage unset",
			config:         &Config{},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			v2qSvcOpts:     &v2querysvc.QueryServiceOptions{},
			expectedOutput: `{"level":"info","msg":"Archive storage not configured"}` + "\n",
			expectedErr:    "",
		},
		{
			name: "Archive storage set",
			config: &Config{
				Storage: Storage{
					TracesArchive: "random-value",
				},
			},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			v2qSvcOpts:     &v2querysvc.QueryServiceOptions{},
			expectedOutput: "",
			expectedErr:    "cannot find traces archive storage factory: cannot find extension",
		},
		{
			name: "Error in trace reader",
			config: &Config{
				Storage: Storage{
					TracesArchive: "need-span-reader-error",
				},
			},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			v2qSvcOpts:     &v2querysvc.QueryServiceOptions{},
			extension:      fakeStorageExt{},
			expectedOutput: "Cannot init traces archive storage reader",
			expectedErr:    "",
		},
		{
			name: "Error in trace writer",
			config: &Config{
				Storage: Storage{
					TracesArchive: "need-span-writer-error",
				},
			},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			v2qSvcOpts:     &v2querysvc.QueryServiceOptions{},
			extension:      fakeStorageExt{},
			expectedOutput: "Cannot init traces archive storage writer",
			expectedErr:    "",
		},
		{
			name: "Archive storage supported",
			config: &Config{
				Storage: Storage{
					TracesArchive: "some-archive-storage",
				},
			},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			v2qSvcOpts:     &v2querysvc.QueryServiceOptions{},
			extension:      fakeStorageExt{},
			expectedOutput: "",
			expectedErr:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := testutils.NewLogger()
			telemetrySettings := component.TelemetrySettings{
				Logger:         logger,
				MeterProvider:  noopmetric.NewMeterProvider(),
				TracerProvider: nooptrace.NewTracerProvider(),
			}
			server := newServer(tt.config, telemetrySettings)
			if tt.extension != nil {
				host = storagetest.NewStorageHost().WithExtension(jaegerstorage.ID, tt.extension)
			}
			err := server.addArchiveStorage(tt.qSvcOpts, tt.v2qSvcOpts, host)
			if tt.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.expectedErr)
			}

			assert.Contains(t, buf.String(), tt.expectedOutput)
		})
	}
}

func TestServerAddMetricsStorage(t *testing.T) {
	host := componenttest.NewNopHost()

	tests := []struct {
		name           string
		config         *Config
		extension      component.Component
		expectedOutput string
		expectedErr    string
	}{
		{
			name:           "Metrics storage unset",
			config:         &Config{},
			expectedOutput: `{"level":"info","msg":"Metric storage not configured"}` + "\n",
			expectedErr:    "",
		},
		{
			name: "Metrics storage set",
			config: &Config{
				Storage: Storage{
					Metrics: "random-value",
				},
			},
			expectedOutput: "",
			expectedErr:    "cannot find metrics storage factory: cannot find extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := testutils.NewLogger()
			telemetrySettings := component.TelemetrySettings{
				Logger:         logger,
				MeterProvider:  noopmetric.NewMeterProvider(),
				TracerProvider: nooptrace.NewTracerProvider(),
			}
			server := newServer(tt.config, telemetrySettings)
			if tt.extension != nil {
				host = storagetest.NewStorageHost().WithExtension(jaegerstorage.ID, tt.extension)
			}
			_, err := server.createMetricReader(host)
			if tt.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.expectedErr)
			}

			assert.Contains(t, buf.String(), tt.expectedOutput)
		})
	}
}
