// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package builder

import (
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	escfg "github.com/uber/jaeger/pkg/es/config"
	"github.com/uber/jaeger/storage/spanstore/memory"
)

// BasicOptions is a set of basic building blocks for most Jaeger executables
type BasicOptions struct {
	// Logger is a generic logger used by most executables
	Logger *zap.Logger
	// MetricsFactory is the basic metrics factory used by most executables
	MetricsFactory metrics.Factory
	// Cassandra is the cassandra configuration used by most executables (if applicable)
	Cassandra *cascfg.Configuration
	// MemoryStore is the memory store (as reader and writer) that will be used if required
	MemoryStore *memory.Store
	// ElasticSearch is the elasticsearch configuration used
	ElasticSearch *escfg.Configuration
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

// CassandraOption creates an Option that adds Cassandra configuration.
func (BasicOptions) CassandraOption(cassandra *cascfg.Configuration) Option {
	return func(b *BasicOptions) {
		b.Cassandra = cassandra
	}
}

// MemoryStoreOption creates an Option that adds a memory store
func (BasicOptions) MemoryStoreOption(memoryStore *memory.Store) Option {
	return func(b *BasicOptions) {
		b.MemoryStore = memoryStore
	}
}

// ElasticSearchOption creates an Option that adds ElasticSearch configuration.
func (BasicOptions) ElasticSearchOption(elastic *escfg.Configuration) Option {
	return func(b *BasicOptions) {
		b.ElasticSearch = elastic
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
