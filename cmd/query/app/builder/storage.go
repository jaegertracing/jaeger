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
	"errors"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	basicB "github.com/jaegertracing/jaeger/cmd/builder"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// StorageBuilder is the interface that provides the necessary store readers
type StorageBuilder struct {
	logger           *zap.Logger
	metricsFactory   metrics.Factory
	SpanReader       spanstore.Reader
	DependencyReader dependencystore.Reader
}

var (
	errMissingCassandraConfig     = errors.New("Cassandra not configured")
	errMissingMemoryStore         = errors.New("Memory Reader was not provided")
	errMissingElasticSearchConfig = errors.New("ElasticSearch not configured")
)

// NewStorageBuilder creates a StorageBuilder based off the flags that have been set
func NewStorageBuilder(storageType string, dependencyDataFreq time.Duration, opts ...basicB.Option) (*StorageBuilder, error) {
	options := basicB.ApplyOptions(opts...)

	sb := &StorageBuilder{
		logger:         options.Logger,
		metricsFactory: options.MetricsFactory,
	}

	// TODO lots of repeated code + if logic, clean up below
	var err error
	if storageType == flags.CassandraStorageType {
		if options.CassandraSessionBuilder == nil {
			return nil, errMissingCassandraConfig
		}
		// TODO technically span and dependency storage might be separate
		err = sb.newCassandraBuilder(options.CassandraSessionBuilder, dependencyDataFreq)
	} else if storageType == flags.MemoryStorageType {
		if options.MemoryStore == nil {
			return nil, errMissingMemoryStore
		}
		sb.newMemoryStoreBuilder(options.MemoryStore)
	} else if storageType == flags.ESStorageType {
		if options.ElasticClientBuilder == nil {
			return nil, errMissingElasticSearchConfig
		}
		err = sb.newESBuilder(options.ElasticClientBuilder)
	} else {
		return nil, flags.ErrUnsupportedStorageType
	}

	if err != nil {
		return nil, err
	}

	return sb, nil
}
