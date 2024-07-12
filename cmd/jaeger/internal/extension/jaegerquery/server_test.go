// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/internal/grpctest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

type fakeFactory struct {
	name string
}

func (ff fakeFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	if ff.name == "need-dependency-reader-error" {
		return nil, fmt.Errorf("test-error")
	}
	return &depsmocks.Reader{}, nil
}

func (ff fakeFactory) CreateSpanReader() (spanstore.Reader, error) {
	if ff.name == "need-span-reader-error" {
		return nil, fmt.Errorf("test-error")
	}
	return &spanstoremocks.Reader{}, nil
}

func (ff fakeFactory) CreateSpanWriter() (spanstore.Writer, error) {
	if ff.name == "need-span-writer-error" {
		return nil, fmt.Errorf("test-error")
	}
	return &spanstoremocks.Writer{}, nil
}

func (ff fakeFactory) Initialize(metrics.Factory, *zap.Logger) error {
	if ff.name == "need-initialize-error" {
		return fmt.Errorf("test-error")
	}
	return nil
}

type fakeStorageExt struct{}

var _ jaegerstorage.Extension = (*fakeStorageExt)(nil)

func (fakeStorageExt) Factory(name string) (storage.Factory, bool) {
	if name == "need-factory-error" {
		return nil, false
	}
	return fakeFactory{name: name}, true
}

func (fakeStorageExt) Start(context.Context, component.Host) error {
	return nil
}

func (fakeStorageExt) Shutdown(context.Context) error {
	return nil
}

type storageHost struct {
	extension component.Component
}

func (storageHost) ReportFatalError(error) {
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		jaegerstorage.ID: host.extension,
	}
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	return nil
}

func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
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
	host := storageHost{
		extension: fakeStorageExt{},
	}
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "Non-empty config with fake storage host",
			config: &Config{
				TraceStorageArchive: "jaeger_storage",
				TraceStoragePrimary: "jaeger_storage",
			},
		},
		{
			name: "factory error",
			config: &Config{
				TraceStoragePrimary: "need-factory-error",
			},
			expectedErr: "cannot find primary storage",
		},
		{
			name: "span reader error",
			config: &Config{
				TraceStoragePrimary: "need-span-reader-error",
			},
			expectedErr: "cannot create span reader",
		},
		{
			name: "dependency error",
			config: &Config{
				TraceStoragePrimary: "need-dependency-reader-error",
			},
			expectedErr: "cannot create dependencies reader",
		},
		{
			name: "storage archive error",
			config: &Config{
				TraceStorageArchive: "need-factory-error",
				TraceStoragePrimary: "jaeger_storage",
			},
			expectedErr: "cannot find archive storage factory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetrySettings := component.TelemetrySettings{
				Logger:       zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
				ReportStatus: func(*component.StatusEvent) {},
			}
			server := newServer(tt.config, telemetrySettings)
			err := server.Start(context.Background(), host)
			if tt.expectedErr == "" {
				require.NoError(t, err)
				// We need to wait for servers to become available.
				// Otherwise, we could call shutdown before the servers are even started,
				// which could cause flaky code coverage by going through error cases.
				require.Eventually(t,
					func() bool {
						resp, err := http.Get("http://localhost:16686/")
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
					HostPort: ":16685",
					ExpectedServices: []string{
						"jaeger.api_v2.QueryService",
						"jaeger.api_v3.QueryService",
						"jaeger.api_v2.metrics.MetricsQueryService",
						"grpc.health.v1.Health",
					},
				}.Execute(t)
				defer server.Shutdown(context.Background())
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
		config         *Config
		extension      component.Component
		expectedOutput string
		expectedErr    string
	}{
		{
			name:           "Archive storage unset",
			config:         &Config{},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			expectedOutput: `{"level":"info","msg":"Archive storage not configured"}` + "\n",
			expectedErr:    "",
		},
		{
			name: "Archive storage set",
			config: &Config{
				TraceStorageArchive: "random-value",
			},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			expectedOutput: "",
			expectedErr:    "cannot find archive storage factory: cannot find extension",
		},
		{
			name: "Archive storage not supported",
			config: &Config{
				TraceStorageArchive: "badger",
			},
			qSvcOpts:       &querysvc.QueryServiceOptions{},
			extension:      fakeStorageExt{},
			expectedOutput: "Archive storage not supported by the factory",
			expectedErr:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := testutils.NewLogger()
			telemetrySettings := component.TelemetrySettings{
				Logger: logger,
			}
			server := newServer(tt.config, telemetrySettings)
			if tt.extension != nil {
				host = storageHost{extension: tt.extension}
			}
			err := server.addArchiveStorage(tt.qSvcOpts, host)
			if tt.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.expectedErr)
			}

			assert.Contains(t, buf.String(), tt.expectedOutput)
		})
	}
}
