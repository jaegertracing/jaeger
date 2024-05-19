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
	"flag"
	"fmt"
	"io"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var ( // interface comformance checks
	_ storage.Factory        = (*Factory)(nil)
	_ storage.ArchiveFactory = (*Factory)(nil)
	_ io.Closer              = (*Factory)(nil)
	_ plugin.Configurable    = (*Factory)(nil)
)

// Factory implements storage.Factory and creates storage components backed by a storage plugin.
type Factory struct {
	options        Options
	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracerProvider trace.TracerProvider

	builder config.PluginBuilder

	store               shared.StoragePlugin
	archiveStore        shared.ArchiveStoragePlugin
	streamingSpanWriter shared.StreamingSpanWriterPlugin
	capabilities        shared.PluginCapabilities
	remoteConn          *grpc.ClientConn
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// NewFactoryWithConfig is used from jaeger(v2).
func NewFactoryWithConfig(
	cfg config.ConfigV2,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	f := NewFactory()
	f.configureFromOptions(Options{ConfigV2: cfg})
	err := f.Initialize(metricsFactory, logger)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := f.options.InitFromViper(v); err != nil {
		logger.Fatal("unable to initialize gRPC storage factory", zap.Error(err))
	}
	f.builder = &f.options.Configuration
}

// configureFromOptions initializes factory from options
func (f *Factory) configureFromOptions(opts Options) {
	f.options = opts
	f.builder = &f.options.Configuration
	f.builder = &f.options.ConfigV2
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	f.tracerProvider = otel.GetTracerProvider()

	var err error
	var services *config.ClientPluginServices
	services, f.remoteConn, err = f.builder.Build(logger, f.tracerProvider)
	if err != nil {
		return fmt.Errorf("grpc storage builder failed to create a store: %w", err)
	}

	f.store = services.Store
	f.archiveStore = services.ArchiveStore
	f.capabilities = services.Capabilities
	f.streamingSpanWriter = services.StreamingSpanWriter
	logger.Info("Remote storage configuration", zap.Any("configuration", f.options.ConfigV2))
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return f.store.SpanReader(), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	if f.capabilities != nil && f.streamingSpanWriter != nil {
		if capabilities, err := f.capabilities.Capabilities(); err == nil && capabilities.StreamingSpanWriter {
			return f.streamingSpanWriter.StreamingSpanWriter(), nil
		}
	}
	return f.store.SpanWriter(), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return f.store.DependencyReader(), nil
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	if f.capabilities == nil {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	capabilities, err := f.capabilities.Capabilities()
	if err != nil {
		return nil, err
	}
	if capabilities == nil || !capabilities.ArchiveSpanReader {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return f.archiveStore.ArchiveSpanReader(), nil
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	if f.capabilities == nil {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	capabilities, err := f.capabilities.Capabilities()
	if err != nil {
		return nil, err
	}
	if capabilities == nil || !capabilities.ArchiveSpanWriter {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return f.archiveStore.ArchiveSpanWriter(), nil
}

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	// TODO Close should move into Services type, instead of being in the Config.
	var errs []error
	if f.remoteConn != nil {
		errs = append(errs, f.remoteConn.Close())
	}
	errs = append(errs, f.options.Configuration.RemoteTLS.Close())
	return errors.Join(errs...)
}
