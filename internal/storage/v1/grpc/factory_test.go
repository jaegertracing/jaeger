// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"log"
	"net"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/grpc/shared"
	"github.com/jaegertracing/jaeger/internal/storage/v1/grpc/shared/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type store struct {
	reader spanstore.Reader
	writer spanstore.Writer
	deps   dependencystore.Reader
}

func (s *store) SpanReader() spanstore.Reader {
	return s.reader
}

func (s *store) SpanWriter() spanstore.Writer {
	return s.writer
}

func (s *store) ArchiveSpanReader() spanstore.Reader {
	return s.reader
}

func (s *store) ArchiveSpanWriter() spanstore.Writer {
	return s.writer
}

func (s *store) DependencyReader() dependencystore.Reader {
	return s.deps
}

func (s *store) StreamingSpanWriter() spanstore.Writer {
	return s.writer
}

func makeMockServices() *ClientPluginServices {
	return &ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store: &store{
				writer: new(spanStoreMocks.Writer),
				reader: new(spanStoreMocks.Reader),
				deps:   new(dependencyStoreMocks.Reader),
			},
			StreamingSpanWriter: &store{
				writer: new(spanStoreMocks.Writer),
			},
		},
		Capabilities: new(mocks.PluginCapabilities),
	}
}

func makeFactory(t *testing.T) *Factory {
	f := NewFactory()
	f.InitFromViper(viper.New(), zap.NewNop())
	f.options.ClientConfig.Endpoint = ":0"
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	t.Cleanup(func() {
		f.Close()
	})

	f.services = makeMockServices()
	return f
}

func TestNewFactoryError(t *testing.T) {
	cfg := &Config{
		ClientConfig: configgrpc.ClientConfig{
			// non-empty Auth is currently not supported
			Auth: &configauth.Authentication{},
		},
	}
	telset := telemetry.NoopSettings()
	t.Run("with_config", func(t *testing.T) {
		_, err := NewFactoryWithConfig(*cfg, telset)
		assert.ErrorContains(t, err, "authenticator")
	})

	t.Run("viper", func(t *testing.T) {
		f := NewFactory()
		f.InitFromViper(viper.New(), zap.NewNop())
		f.options.Config = *cfg
		err := f.Initialize(metrics.NullFactory, zap.NewNop())
		assert.ErrorContains(t, err, "authenticator")
	})

	t.Run("client", func(t *testing.T) {
		// this is a silly test to verify handling of error from grpc.NewClient, which cannot be induced via params.
		f, err := NewFactoryWithConfig(Config{
			ClientConfig: configgrpc.ClientConfig{
				Endpoint: ":0",
			},
		}, telset)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, f.Close()) })
		newClientFn := func(_ component.TelemetrySettings, _ ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
			return nil, errors.New("test error")
		}
		_, err = f.newRemoteStorage(component.TelemetrySettings{}, component.TelemetrySettings{}, newClientFn)
		assert.ErrorContains(t, err, "error creating traced remote storage client")
	})
}

func TestInitFactory(t *testing.T) {
	f := makeFactory(t)
	f.services.Capabilities = nil

	reader, err := f.CreateSpanReader()
	require.NoError(t, err)
	assert.NotNil(t, reader)

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, f.services.Store.SpanWriter(), writer)

	depReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	assert.Equal(t, f.services.Store.DependencyReader(), depReader)
}

func TestGRPCStorageFactoryWithConfig(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")

	s := grpc.NewServer()
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
	defer s.Stop()

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: lis.Addr().String(),
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 1 * time.Second,
		},
		Tenancy: tenancy.Options{
			Enabled: true,
		},
	}
	telset := telemetry.NoopSettings()
	f, err := NewFactoryWithConfig(cfg, telset)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestGRPCStorageFactory_Capabilities(t *testing.T) {
	f := makeFactory(t)

	capabilities := f.services.Capabilities.(*mocks.PluginCapabilities)
	capabilities.On("Capabilities").
		Return(&shared.Capabilities{
			StreamingSpanWriter: true,
		}, nil).Times(1)

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.NotNil(t, writer)
}

func TestGRPCStorageFactory_CapabilitiesDisabled(t *testing.T) {
	f := makeFactory(t)

	capabilities := f.services.Capabilities.(*mocks.PluginCapabilities)
	capabilities.On("Capabilities").
		Return(&shared.Capabilities{
			StreamingSpanWriter: false,
		}, nil).Times(1)

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.NotNil(t, writer, "regular span writer is available")
}

func TestGRPCStorageFactory_CapabilitiesError(t *testing.T) {
	f := makeFactory(t)

	capabilities := f.services.Capabilities.(*mocks.PluginCapabilities)
	customError := errors.New("made-up error")
	capabilities.On("Capabilities").Return(nil, customError)

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.NotNil(t, writer, "regular span writer is available")
}

func TestGRPCStorageFactory_CapabilitiesNil(t *testing.T) {
	f := makeFactory(t)
	f.services.Capabilities = nil

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.NotNil(t, writer, "regular span writer is available")
}

func TestWithCLIFlags(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=foo:1234",
	})
	require.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())
	assert.Equal(t, "foo:1234", f.options.Config.ClientConfig.Endpoint)
	require.NoError(t, f.Close())
}

func TestStreamingSpanWriterFactory_CapabilitiesNil(t *testing.T) {
	f := makeFactory(t)

	f.services.Capabilities = nil
	mockWriter := f.services.Store.SpanWriter().(*spanStoreMocks.Writer)
	mockWriter.On("WriteSpan", mock.Anything, mock.Anything).Return(errors.New("not streaming writer"))
	mockWriter2 := f.services.StreamingSpanWriter.StreamingSpanWriter().(*spanStoreMocks.Writer)
	mockWriter2.On("WriteSpan", mock.Anything, mock.Anything).Return(errors.New("I am streaming writer"))

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	err = writer.WriteSpan(context.Background(), nil)
	assert.ErrorContains(t, err, "not streaming writer")
}

func TestStreamingSpanWriterFactory_Capabilities(t *testing.T) {
	f := makeFactory(t)

	capabilities := f.services.Capabilities.(*mocks.PluginCapabilities)
	customError := errors.New("made-up error")
	capabilities.
		// return error on the first call
		On("Capabilities").Return(nil, customError).Once().
		// then return false on the second call
		On("Capabilities").Return(&shared.Capabilities{}, nil).Once().
		// then return true on the second call
		On("Capabilities").Return(&shared.Capabilities{StreamingSpanWriter: true}, nil).Once()

	mockWriter := f.services.Store.SpanWriter().(*spanStoreMocks.Writer)
	mockWriter.On("WriteSpan", mock.Anything, mock.Anything).Return(errors.New("not streaming writer"))
	mockWriter2 := f.services.StreamingSpanWriter.StreamingSpanWriter().(*spanStoreMocks.Writer)
	mockWriter2.On("WriteSpan", mock.Anything, mock.Anything).Return(errors.New("I am streaming writer"))

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	err = writer.WriteSpan(context.Background(), nil)
	require.ErrorContains(t, err, "not streaming writer", "unary writer when Capabilities return error")

	writer, err = f.CreateSpanWriter()
	require.NoError(t, err)
	err = writer.WriteSpan(context.Background(), nil)
	require.ErrorContains(t, err, "not streaming writer", "unary writer when Capabilities return false")

	writer, err = f.CreateSpanWriter()
	require.NoError(t, err)
	err = writer.WriteSpan(context.Background(), nil)
	assert.ErrorContains(t, err, "I am streaming writer", "streaming writer when Capabilities return true")
}

func TestIsArchiveCapable(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		enabled   bool
		expected  bool
	}{
		{
			name:      "archive capable",
			namespace: "grpc-storage-archive",
			enabled:   true,
			expected:  true,
		},
		{
			name:      "not capable",
			namespace: "grpc-storage-archive",
			enabled:   false,
			expected:  false,
		},
		{
			name:      "capable + wrong namespace",
			namespace: "grpc-storage",
			enabled:   true,
			expected:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := &Factory{
				options: &options{
					namespace: test.namespace,
					Config: Config{
						enabled: test.enabled,
					},
				},
			}
			result := factory.IsArchiveCapable()
			require.Equal(t, test.expected, result)
		})
	}
}
