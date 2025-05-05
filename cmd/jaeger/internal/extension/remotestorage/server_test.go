// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confignet"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/grpctest"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

type fakeTraceStorageFactory struct {
	name string
}

func (ff fakeTraceStorageFactory) CreateTraceReader() (tracestore.Reader, error) {
	if ff.name == "need-trace-reader-error" {
		return nil, assert.AnError
	}
	return &tracestoremocks.Reader{}, nil
}

func (ff fakeTraceStorageFactory) CreateTraceWriter() (tracestore.Writer, error) {
	if ff.name == "need-span-writer-error" {
		return nil, assert.AnError
	}
	return &tracestoremocks.Writer{}, nil
}

type fakeDependenciesStorageFactory struct {
	name string
}

func (ff fakeDependenciesStorageFactory) CreateDependencyReader() (depstore.Reader, error) {
	if ff.name == "need-dependency-reader-error" {
		return nil, assert.AnError
	}
	return &depstoremocks.Reader{}, nil
}

type fakeFactory struct {
	fakeTraceStorageFactory
	fakeDependenciesStorageFactory
}

func newFakeFactory(name string) *fakeFactory {
	return &fakeFactory{
		fakeTraceStorageFactory:        fakeTraceStorageFactory{name: name},
		fakeDependenciesStorageFactory: fakeDependenciesStorageFactory{name: name},
	}
}

type fakeStorageExt struct{}

var _ jaegerstorage.Extension = (*fakeStorageExt)(nil)

func (fakeStorageExt) TraceStorageFactory(name string) (tracestore.Factory, bool) {
	switch name {
	case "need-factory-error":
		return nil, false
	case "without-dependency-storage":
		return fakeTraceStorageFactory{name: name}, true
	}

	return newFakeFactory(name), true
}

func (fakeStorageExt) MetricStorageFactory(string) (storage.MetricStoreFactory, bool) {
	return nil, false
}

func (fakeStorageExt) Start(context.Context, component.Host) error {
	return nil
}

func (fakeStorageExt) Shutdown(context.Context) error {
	return nil
}

func TestServer_Dependencies(t *testing.T) {
	expectedDependencies := []component.ID{jaegerstorage.ID}
	telemetrySettings := component.TelemetrySettings{
		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
	}

	server := newServer(createDefaultConfig().(*Config), telemetrySettings)
	dependencies := server.Dependencies()

	require.Equal(t, expectedDependencies, dependencies)
}

func TestServer_Start(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(jaegerstorage.ID, fakeStorageExt{})
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "Real server with non-empty config",
			config: &Config{
				Storage: "jaeger_storage",
			},
		},
		{
			name: "no trace storage factory",
			config: &Config{
				Storage: "need-factory-error",
			},
			expectedErr: "cannot find factory for trace storage",
		},
		{
			name: "no dependency storage factory",
			config: &Config{
				Storage: "without-dependency-storage",
			},
			expectedErr: "cannot find factory for dependency storage",
		},
		{
			name: "error creating server",
			config: &Config{
				Storage: "need-trace-reader-error",
			},
			expectedErr: "could not create remote storage server",
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
			tt.config.NetAddr.Endpoint = "localhost:0"
			tt.config.NetAddr.Transport = confignet.TransportTypeTCP
			server := newServer(tt.config, telemetrySettings)
			err := server.Start(context.Background(), host)
			if tt.expectedErr == "" {
				require.NoError(t, err)
				defer server.Shutdown(context.Background())
				grpctest.ReflectionServiceValidator{
					HostPort: server.server.GRPCAddr(),
					ExpectedServices: []string{
						"jaeger.storage.v2.TraceReader",
						"jaeger.storage.v2.DependencyReader",
						"grpc.health.v1.Health",
					},
				}.Execute(t)
			} else {
				require.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}
