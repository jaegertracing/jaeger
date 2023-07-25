// Copyright (c) 2021 The Jaeger Authors.
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

package prometheus

import (
	"flag"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin"
	prometheusstore "github.com/jaegertracing/jaeger/plugin/metrics/prometheus/metricsstore"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

var _ plugin.Configurable = (*Factory)(nil)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	options *Options
	logger  *zap.Logger
	tracer  trace.TracerProvider
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		tracer:  otel.GetTracerProvider(),
		options: NewOptions("prometheus"),
	}
}

// AddFlags implements plugin.Configurable.
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable.
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := f.options.InitFromViper(v); err != nil {
		logger.Panic("Failed to initialize metrics storage factory", zap.Error(err))
	}
}

// Initialize implements storage.MetricsFactory.
func (f *Factory) Initialize(logger *zap.Logger) error {
	f.logger = logger
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricsstore.Reader, error) {
	return prometheusstore.NewMetricsReader(f.options.Primary.Configuration, f.logger, f.tracer)
}
