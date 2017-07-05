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
	"errors"
	"flag"

	basicB "github.com/uber/jaeger/cmd/builder"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/storage/dependencystore"
	"github.com/uber/jaeger/storage/spanstore"
)

// StorageBuilder is the interface that provides the necessary store readers
type StorageBuilder interface {
	NewSpanReader() (spanstore.Reader, error)
	NewDependencyReader() (dependencystore.Reader, error)
}

var (
	errMissingCassandraConfig     = errors.New("Cassandra not configured")
	errMissingMemoryStore         = errors.New("Memory Reader was not provided")
	errMissingElasticSearchConfig = errors.New("ElasticSearch not configured")
)

// NewStorageBuilder creates a StorageBuilder based off the flags that have been set
func NewStorageBuilder(opts ...basicB.Option) (StorageBuilder, error) {
	flag.Parse()
	options := basicB.ApplyOptions(opts...)
	if flags.SpanStorage.Type == flags.CassandraStorageType {
		if options.Cassandra == nil {
			return nil, errMissingCassandraConfig
		}
		// TODO technically span and dependency storage might be separate
		return newCassandraBuilder(options.Cassandra, options.Logger, options.MetricsFactory), nil
	} else if flags.SpanStorage.Type == flags.MemoryStorageType {
		if options.MemoryStore == nil {
			return nil, errMissingMemoryStore
		}
		return newMemoryStoreBuilder(options.MemoryStore), nil
	} else if flags.SpanStorage.Type == flags.ESStorageType {
		if options.Elastic == nil {
			return nil, errMissingElasticSearchConfig
		}
		return newESBuilder(options.Elastic, options.Logger), nil
	}
	return nil, flags.ErrUnsupportedStorageType
}
