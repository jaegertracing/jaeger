// Copyright (c) 2023 The Jaeger Authors.
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
	"flag"
	"fmt"
	"io"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

var (
	_ plugin.Configurable = (*Factory)(nil)
	_ io.Closer           = (*Factory)(nil)
)

// Factory implements storage.MetricsFactory
type Factory struct {
	options Options
	logger  *zap.Logger

	builder config.PluginBuilder

	metricsReader shared.MetricsReaderPlugin
	capabilities  shared.PluginCapabilities
}

// NewFactory creates a new Factory
func NewFactory() *Factory {
	return &Factory{}
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := f.options.InitFromViper(v); err != nil {
		logger.Fatal("unable to initialize gRPC metrics factory", zap.Error(err))
	}
	f.builder = &f.options.Configuration
}

// Initialize implements storage.MetricsFactory
func (f *Factory) Initialize(logger *zap.Logger) error {
	f.logger = logger

	services, err := f.builder.Build(logger)
	if err != nil {
		return fmt.Errorf("grpc-plugin builder failed to create a store: %w", err)
	}

	f.metricsReader = services.MetricsReader
	f.capabilities = services.Capabilities
	logger.Info("External plugin storage configuration", zap.Any("configuration", f.options.Configuration))
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory
func (f *Factory) CreateMetricsReader() (metricsstore.Reader, error) {
	if f.capabilities == nil {
		return nil, storage.ErrMetricsStorageNotSupported
	}
	capabilities, err := f.capabilities.Capabilities()
	if err != nil {
		return nil, err
	}
	if capabilities == nil || !capabilities.MetricsReader {
		return nil, storage.ErrMetricsStorageNotSupported
	}
	return f.metricsReader.MetricsReader(), nil
}

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	return f.builder.Close()
}
