// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpc

import (
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	grpcConfig "github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/mocks"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var _ storage.Factory = new(Factory)

type mockPluginBuilder struct {
	plugin     *mockPlugin
	writerType string
	err        error
}

func (b *mockPluginBuilder) Build(logger *zap.Logger, tracer trace.TracerProvider) (*grpcConfig.ClientPluginServices, error) {
	if b.err != nil {
		return nil, b.err
	}

	services := &grpcConfig.ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store:        b.plugin,
			ArchiveStore: b.plugin,
		},
	}
	if b.writerType == "streaming" {
		services.PluginServices.StreamingSpanWriter = b.plugin
	}
	if b.plugin.capabilities != nil {
		services.Capabilities = b.plugin
	}

	return services, nil
}

func (b *mockPluginBuilder) Close() error {
	return nil
}

type mockPlugin struct {
	spanReader          spanstore.Reader
	spanWriter          spanstore.Writer
	archiveReader       spanstore.Reader
	archiveWriter       spanstore.Writer
	streamingSpanWriter spanstore.Writer
	capabilities        shared.PluginCapabilities
	dependencyReader    dependencystore.Reader
}

func (mp *mockPlugin) Capabilities() (*shared.Capabilities, error) {
	return mp.capabilities.Capabilities()
}

func (mp *mockPlugin) ArchiveSpanReader() spanstore.Reader {
	return mp.archiveReader
}

func (mp *mockPlugin) ArchiveSpanWriter() spanstore.Writer {
	return mp.archiveWriter
}

func (mp *mockPlugin) SpanReader() spanstore.Reader {
	return mp.spanReader
}

func (mp *mockPlugin) SpanWriter() spanstore.Writer {
	return mp.spanWriter
}

func (mp *mockPlugin) StreamingSpanWriter() spanstore.Writer {
	return mp.streamingSpanWriter
}

func (mp *mockPlugin) DependencyReader() dependencystore.Reader {
	return mp.dependencyReader
}

func TestGRPCStorageFactory(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v, zap.NewNop())

	// after InitFromViper, f.builder points to a real plugin builder that will fail in unit tests,
	// so we override it with a mock.
	f.builder = &mockPluginBuilder{
		err: errors.New("made-up error"),
	}
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "made-up error")

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{
			spanWriter:       new(spanStoreMocks.Writer),
			spanReader:       new(spanStoreMocks.Reader),
			archiveWriter:    new(spanStoreMocks.Writer),
			archiveReader:    new(spanStoreMocks.Reader),
			capabilities:     new(mocks.PluginCapabilities),
			dependencyReader: new(dependencyStoreMocks.Reader),
		},
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	assert.NotNil(t, f.store)
	reader, err := f.CreateSpanReader()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanReader(), reader)
	writer, err := f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanWriter(), writer)
	depReader, err := f.CreateDependencyReader()
	assert.NoError(t, err)
	assert.Equal(t, f.store.DependencyReader(), depReader)
}

func TestGRPCStorageFactory_Capabilities(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v, zap.NewNop())

	capabilities := new(mocks.PluginCapabilities)
	capabilities.On("Capabilities").
		Return(&shared.Capabilities{
			ArchiveSpanReader:   true,
			ArchiveSpanWriter:   true,
			StreamingSpanWriter: true,
		}, nil).Times(3)

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{
			capabilities:        capabilities,
			archiveWriter:       new(spanStoreMocks.Writer),
			archiveReader:       new(spanStoreMocks.Reader),
			streamingSpanWriter: new(spanStoreMocks.Writer),
		},
		writerType: "streaming",
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	assert.NotNil(t, f.store)
	reader, err := f.CreateArchiveSpanReader()
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	writer, err := f.CreateArchiveSpanWriter()
	assert.NoError(t, err)
	assert.NotNil(t, writer)
	writer, err = f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.streamingSpanWriter.StreamingSpanWriter(), writer)
}

func TestGRPCStorageFactory_CapabilitiesDisabled(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v, zap.NewNop())

	capabilities := new(mocks.PluginCapabilities)
	capabilities.On("Capabilities").
		Return(&shared.Capabilities{
			ArchiveSpanReader:   false,
			ArchiveSpanWriter:   false,
			StreamingSpanWriter: false,
		}, nil)

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{
			capabilities:  capabilities,
			archiveWriter: new(spanStoreMocks.Writer),
			archiveReader: new(spanStoreMocks.Reader),
			spanWriter:    new(spanStoreMocks.Writer),
		},
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	assert.NotNil(t, f.store)
	reader, err := f.CreateArchiveSpanReader()
	assert.EqualError(t, err, storage.ErrArchiveStorageNotSupported.Error())
	assert.Nil(t, reader)
	writer, err := f.CreateArchiveSpanWriter()
	assert.EqualError(t, err, storage.ErrArchiveStorageNotSupported.Error())
	assert.Nil(t, writer)
	writer, err = f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanWriter(), writer)
}

func TestGRPCStorageFactory_CapabilitiesError(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v, zap.NewNop())

	capabilities := new(mocks.PluginCapabilities)
	customError := errors.New("made-up error")
	capabilities.On("Capabilities").
		Return(nil, customError)

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{
			capabilities:  capabilities,
			archiveWriter: new(spanStoreMocks.Writer),
			archiveReader: new(spanStoreMocks.Reader),
			spanWriter:    new(spanStoreMocks.Writer),
		},
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	assert.NotNil(t, f.store)
	reader, err := f.CreateArchiveSpanReader()
	assert.EqualError(t, err, customError.Error())
	assert.Nil(t, reader)
	writer, err := f.CreateArchiveSpanWriter()
	assert.EqualError(t, err, customError.Error())
	assert.Nil(t, writer)
	writer, err = f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanWriter(), writer)
}

func TestGRPCStorageFactory_CapabilitiesNil(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v, zap.NewNop())

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{
			archiveWriter: new(spanStoreMocks.Writer),
			archiveReader: new(spanStoreMocks.Reader),
			spanWriter:    new(spanStoreMocks.Writer),
		},
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	assert.NotNil(t, f.store)
	reader, err := f.CreateArchiveSpanReader()
	assert.Equal(t, err, storage.ErrArchiveStorageNotSupported)
	assert.Nil(t, reader)
	writer, err := f.CreateArchiveSpanWriter()
	assert.Equal(t, err, storage.ErrArchiveStorageNotSupported)
	assert.Nil(t, writer)
	writer, err = f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanWriter(), writer)
}

func TestWithConfiguration(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage-plugin.log-level=debug",
		"--grpc-storage-plugin.binary=noop-grpc-plugin",
		"--grpc-storage-plugin.configuration-file=config.json",
	})
	assert.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())
	assert.Equal(t, f.options.Configuration.PluginBinary, "noop-grpc-plugin")
	assert.Equal(t, f.options.Configuration.PluginConfigurationFile, "config.json")
	assert.Equal(t, f.options.Configuration.PluginLogLevel, "debug")
	assert.NoError(t, f.Close())
}

func TestInitFromOptions(t *testing.T) {
	f := Factory{}
	o := Options{
		Configuration: grpcConfig.Configuration{
			PluginLogLevel: "info",
		},
	}
	f.InitFromOptions(o)
	assert.Equal(t, o, f.options)
	assert.Equal(t, &o.Configuration, f.builder)
}

func TestStreamingSpanWriterFactory_CapabilitiesNil(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v, zap.NewNop())

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{
			archiveWriter: new(spanStoreMocks.Writer),
			archiveReader: new(spanStoreMocks.Reader),
			spanWriter:    new(spanStoreMocks.Writer),
		},
		writerType: "streaming",
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	writer, err := f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanWriter(), writer)
}

func TestStreamingSpanWriterFactory_Capabilities(t *testing.T) {
	f := NewFactory()
	v := viper.New()
	f.InitFromViper(v, zap.NewNop())

	capabilities := new(mocks.PluginCapabilities)
	customError := errors.New("made-up error")
	capabilities.On("Capabilities").
		Return(nil, customError).Once().
		On("Capabilities").Return(&shared.Capabilities{}, nil).Once()

	f.builder = &mockPluginBuilder{
		plugin: &mockPlugin{
			archiveWriter: new(spanStoreMocks.Writer),
			archiveReader: new(spanStoreMocks.Reader),
			spanWriter:    new(spanStoreMocks.Writer),
			capabilities:  capabilities,
		},
		writerType: "streaming",
	}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	writer, err := f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanWriter(), writer) // get unary writer when Capabilities return error

	writer, err = f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, f.store.SpanWriter(), writer) // get unary writer when Capabilities return false
}
