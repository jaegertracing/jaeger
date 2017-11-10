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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	cascfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	escfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/storage/spanstore/memory"
	"github.com/uber/jaeger-lib/metrics"
)

func TestApplyOptions(t *testing.T) {
	opts := ApplyOptions(
		Options.CassandraSessionOption(&cascfg.Configuration{}),
		Options.LoggerOption(zap.NewNop()),
		Options.MetricsFactoryOption(metrics.NullFactory),
		Options.MemoryStoreOption(memory.NewStore()),
		Options.ElasticClientOption(&escfg.Configuration{
			Servers: []string{"127.0.0.1"},
		}),
	)
	assert.NotNil(t, opts.CassandraSessionBuilder)
	assert.NotNil(t, opts.ElasticClientBuilder)
	assert.NotNil(t, opts.Logger)
	assert.NotNil(t, opts.MetricsFactory)
}

func TestApplyNoOptions(t *testing.T) {
	opts := ApplyOptions()
	assert.NotNil(t, opts.Logger)
	assert.NotNil(t, opts.MetricsFactory)
}
