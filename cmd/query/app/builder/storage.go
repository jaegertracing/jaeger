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
	"time"

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
func NewStorageBuilder(storageType string, dependencyDataFreq time.Duration, opts ...basicB.Option) (StorageBuilder, error) {
	options := basicB.ApplyOptions(opts...)
	// TODO lots of repeated code + if logic, clean up below
	if storageType == flags.CassandraStorageType {
		if options.CassandraSessionBuilder == nil {
			return nil, errMissingCassandraConfig
		}
		// TODO technically span and dependency storage might be separate
		return newCassandraBuilder(options.CassandraSessionBuilder, options.Logger, options.MetricsFactory, dependencyDataFreq)
	} else if storageType == flags.MemoryStorageType {
		if options.MemoryStore == nil {
			return nil, errMissingMemoryStore
		}
		return newMemoryStoreBuilder(options.MemoryStore), nil
	} else if storageType == flags.ESStorageType {
		if options.ElasticSearch == nil {
			return nil, errMissingElasticSearchConfig
		}
		return newESBuilder(options.ElasticSearch, options.Logger, options.MetricsFactory), nil
	}
	return nil, flags.ErrUnsupportedStorageType
}
