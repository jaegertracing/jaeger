// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package disabled

import (
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

var _ plugin.Configurable = (*Factory)(nil)

// Factory implements storage.Factory that returns a Disabled metrics reader.
type Factory struct{}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// AddFlags implements plugin.Configurable.
func (*Factory) AddFlags(_ *flag.FlagSet) {}

// InitFromViper implements plugin.Configurable.
func (*Factory) InitFromViper(_ *viper.Viper, _ *zap.Logger) {}

// Initialize implements storage.MetricsFactory.
func (*Factory) Initialize(_ telemetry.Settings) error {
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (*Factory) CreateMetricsReader() (metricsstore.Reader, error) {
	return NewMetricsReader()
}
