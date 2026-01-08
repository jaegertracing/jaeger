// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"errors"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

type fakeFactory struct {
	name string
}

func (fakeFactory) CreateDependencyReader() (depstore.Reader, error) {
	return &depstoremocks.Reader{}, nil
}

func (ff fakeFactory) CreateTraceReader() (tracestore.Reader, error) {
	if ff.name == "need-trace-reader-error" {
		return nil, errors.New("test-error")
	}
	return &tracestoremocks.Reader{}, nil
}

func (fakeFactory) CreateTraceWriter() (tracestore.Writer, error) {
	return &tracestoremocks.Writer{}, nil
}

type fakeStorageExt struct{}

var _ jaegerstorage.Extension = (*fakeStorageExt)(nil)

func (fakeStorageExt) TraceStorageFactory(name string) (tracestore.Factory, bool) {
	if name == "need-factory-error" {
		return nil, false
	}
	return fakeFactory{name: name}, true
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

func TestServerLifecycle(t *testing.T) {
	host := storagetest.NewStorageHost().WithExtension(jaegerstorage.ID, fakeStorageExt{})

	tests := []struct {
		name          string
		config        *Config
		expectedError string
	}{
		{
			name: "successful start and shutdown",
			config: &Config{
				Storage: Storage{
					Traces: "teststorage",
				},
				HTTP: createDefaultConfig().(*Config).HTTP,
			},
			expectedError: "",
		},
		{
			name: "missing storage factory",
			config: &Config{
				Storage: Storage{
					Traces: "need-factory-error",
				},
				HTTP: createDefaultConfig().(*Config).HTTP,
			},
			expectedError: "cannot find factory for trace storage",
		},
		{
			name: "trace reader error",
			config: &Config{
				Storage: Storage{
					Traces: "need-trace-reader-error",
				},
				HTTP: createDefaultConfig().(*Config).HTTP,
			},
			expectedError: "cannot create trace reader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telset := componenttest.NewNopTelemetrySettings()
			server := newServer(tt.config, telset)
			require.NotNil(t, server)

			// Test Start
			err := server.Start(context.Background(), host)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}
			require.NoError(t, err)

			// Test Shutdown
			err = server.Shutdown(context.Background())
			assert.NoError(t, err)
		})
	}
}

func TestServerDependencies(t *testing.T) {
	server := &server{}
	deps := server.Dependencies()
	require.Len(t, deps, 1)
	assert.Equal(t, jaegerstorage.ID, deps[0])
}

func TestShutdownWithoutStart(t *testing.T) {
	telset := componenttest.NewNopTelemetrySettings()
	server := newServer(createDefaultConfig().(*Config), telset)

	err := server.Shutdown(context.Background())
	assert.NoError(t, err)
}
