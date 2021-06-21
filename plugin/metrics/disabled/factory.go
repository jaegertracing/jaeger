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

package disabled

import (
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

// Factory implements storage.Factory that returns a Disabled metrics reader.
type Factory struct{}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// AddFlags implements plugin.Configurable.
func (f *Factory) AddFlags(_ *flag.FlagSet) {}

// InitFromViper implements plugin.Configurable.
func (f *Factory) InitFromViper(_ *viper.Viper, _ *zap.Logger) {}

// Initialize implements storage.MetricsFactory.
func (f *Factory) Initialize(_ *zap.Logger) error {
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricsstore.Reader, error) {
	return NewMetricsReader()
}
