// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

type storageHost struct{}

func (host storageHost) ReportFatalError(err error) {
}

func (host storageHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{
		component.NewID("jaeger_storage"): makeStorageExtension("jaeger_storage"),
	}
}

func (storageHost) GetFactory(_ component.Kind, _ component.Type) component.Factory {
	return nil
}

func (storageHost) GetExporters() map[component.DataType]map[component.ID]component.Component {
	return nil
}

func Test_Dependencies(t *testing.T) {
	expectedDependencies := []component.ID{jaegerstorage.ID}
	telemetrySettings := component.TelemetrySettings{
		Logger: zap.NewNop(),
	}

	server := newServer(createDefaultConfig().(*Config), telemetrySettings)
	dependencies := server.Dependencies()

	assert.Equal(t, expectedDependencies, dependencies)
}

func Test_Start(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		host        component.Host
		logger      *zap.Logger
		expectedErr string
	}{
		{
			name:        "Empty config",
			config:      &Config{},
			host:        componenttest.NewNopHost(),
			logger:      zap.NewNop(),
			expectedErr: "cannot find primary storage : cannot find extension",
		},
		{
			name: "Non-empty config with custom storage host",
			config: &Config{
				TraceStorageArchive: "jaeger_storage",
				TraceStoragePrimary: "jaeger_storage",
			},
			host:        storageHost{},
			logger:      zap.NewNop(),
			expectedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetrySettings := component.TelemetrySettings{
				Logger: tt.logger,
			}
			server := newServer(tt.config, telemetrySettings)
			err := server.Start(context.Background(), tt.host)

			if tt.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}

func Test_AddArchiveStorage(t *testing.T) {
	host := componenttest.NewNopHost()

	tests := []struct {
		name           string
		qSvcOpts       *querysvc.QueryServiceOptions
		config         *Config
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
			expectedErr:    "cannot find archive storage factory: cannot find extension 'jaeger_storage' (make sure it's defined earlier in the config)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := testutils.NewLogger()
			telemetrySettings := component.TelemetrySettings{
				Logger: logger,
			}
			server := newServer(tt.config, telemetrySettings)
			err := server.addArchiveStorage(tt.qSvcOpts, host)
			if tt.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.expectedErr)
			}

			assert.Equal(t, tt.expectedOutput, buf.String())
		})
	}
}

func Test_Shutdown(t *testing.T) {
	tests := []struct {
		name           string
		hasQueryServer bool
	}{
		{
			name:           "server.server nil",
			hasQueryServer: false,
		},
		{
			name:           "server.server not nil",
			hasQueryServer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetrySettings := component.TelemetrySettings{
				Logger: zap.NewNop(),
			}
			server := newServer(createDefaultConfig().(*Config), telemetrySettings)
			assert.NotNil(t, server)

			if tt.hasQueryServer {
				serverOptions := &queryApp.QueryOptions{
					HTTPHostPort: ":8080",
					GRPCHostPort: ":8080",
					QueryOptionsBase: queryApp.QueryOptionsBase{
						Tenancy: tenancy.Options{
							Enabled: true,
						},
					},
				}
				tenancyMgr := tenancy.NewManager(&serverOptions.Tenancy)
				spanReader := &spanstoremocks.Reader{}
				dependencyReader := &depsmocks.Reader{}
				querySvc := querysvc.NewQueryService(spanReader, dependencyReader, querysvc.QueryServiceOptions{})
				queryAppServer, err := queryApp.NewServer(zap.NewNop(), querySvc, nil,
					serverOptions, tenancyMgr,
					jtracer.NoOp())
				require.NoError(t, err)

				err = queryAppServer.Start()
				require.NoError(t, err)

				server.server = queryAppServer
			}

			err := server.Shutdown(context.Background())
			require.NoError(t, err)
		})
	}
}

func makeStorageExtension(memstoreName string) component.Component {
	extensionFactory := jaegerstorage.NewFactory()

	ctx := context.Background()
	telemetrySettings := component.TelemetrySettings{
		Logger:         zap.L(),
		TracerProvider: nooptrace.NewTracerProvider(),
		MeterProvider:  noopmetric.NewMeterProvider(),
	}
	config := &jaegerstorage.Config{
		Memory: map[string]memoryCfg.Configuration{
			memstoreName: {MaxTraces: 10000},
		},
	}

	storageExtension, _ := extensionFactory.CreateExtension(ctx, extension.CreateSettings{
		ID:                ID,
		TelemetrySettings: telemetrySettings,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}, config)

	host := componenttest.NewNopHost()
	storageExtension.Start(ctx, host)

	return storageExtension
}
