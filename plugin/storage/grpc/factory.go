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

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
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
	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracerProvider trace.TracerProvider

	// configV1 is used for backward compatibility. it will be removed in v2.
	// In the main initialization logic, only configV2 is used.
	configV1 Configuration
	configV2 *ConfigV2

	services *ClientPluginServices
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// NewFactoryWithConfig is used from jaeger(v2).
func NewFactoryWithConfig(
	cfg ConfigV2,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	f := NewFactory()
	f.configV2 = &cfg
	if err := f.Initialize(metricsFactory, logger); err != nil {
		return nil, err
	}
	return f, nil
}

// AddFlags implements plugin.Configurable
func (*Factory) AddFlags(flagSet *flag.FlagSet) {
	v1AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := v1InitFromViper(&f.configV1, v); err != nil {
		logger.Fatal("unable to initialize gRPC storage factory", zap.Error(err))
	}
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	f.tracerProvider = otel.GetTracerProvider()

	if f.configV2 == nil {
		f.configV2 = f.configV1.TranslateToConfigV2()
	}

	var err error
	f.services, err = f.configV2.Build(logger, f.tracerProvider)
	if err != nil {
		return fmt.Errorf("grpc storage builder failed to create a store: %w", err)
	}
	logger.Info("Remote storage configuration", zap.Any("configuration", f.configV2))
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return f.services.Store.SpanReader(), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	if f.services.Capabilities != nil && f.services.StreamingSpanWriter != nil {
		if capabilities, err := f.services.Capabilities.Capabilities(); err == nil && capabilities.StreamingSpanWriter {
			return f.services.StreamingSpanWriter.StreamingSpanWriter(), nil
		}
	}
	return f.services.Store.SpanWriter(), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return f.services.Store.DependencyReader(), nil
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	if f.services.Capabilities == nil {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	capabilities, err := f.services.Capabilities.Capabilities()
	if err != nil {
		return nil, err
	}
	if capabilities == nil || !capabilities.ArchiveSpanReader {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return f.services.ArchiveStore.ArchiveSpanReader(), nil
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	if f.services.Capabilities == nil {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	capabilities, err := f.services.Capabilities.Capabilities()
	if err != nil {
		return nil, err
	}
	if capabilities == nil || !capabilities.ArchiveSpanWriter {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return f.services.ArchiveStore.ArchiveSpanWriter(), nil
}

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	var errs []error
	if f.services != nil {
		errs = append(errs, f.services.Close())
	}
	errs = append(errs, f.configV1.RemoteTLS.Close())
	return errors.Join(errs...)
}
