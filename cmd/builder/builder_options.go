// Copyright (c) 2017 Uber Technologies, Inc.
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

package builder

import (
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
)

// TODO nothing but Collector uses this, move to cmd/collector/builder

// BasicOptions is a set of basic building blocks for most Jaeger executables
type BasicOptions struct {
	// Logger is a generic logger used by most executables
	Logger *zap.Logger
	// MetricsFactory is the basic metrics factory used by most executables
	MetricsFactory metrics.Factory
}

// Option is a function that sets some option on StorageBuilder.
type Option func(c *BasicOptions)

// Options is a factory for all available Option's
var Options BasicOptions

// LoggerOption creates an Option that initializes the logger
func (BasicOptions) LoggerOption(logger *zap.Logger) Option {
	return func(b *BasicOptions) {
		b.Logger = logger
	}
}

// MetricsFactoryOption creates an Option that initializes the MetricsFactory
func (BasicOptions) MetricsFactoryOption(metricsFactory metrics.Factory) Option {
	return func(b *BasicOptions) {
		b.MetricsFactory = metricsFactory
	}
}

// ApplyOptions takes a set of options and creates a populated BasicOptions struct
func ApplyOptions(opts ...Option) BasicOptions {
	o := BasicOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.Logger == nil {
		o.Logger = zap.NewNop()
	}
	if o.MetricsFactory == nil {
		o.MetricsFactory = metrics.NullFactory
	}
	return o
}
