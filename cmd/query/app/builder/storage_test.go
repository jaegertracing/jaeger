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
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	basicB "github.com/jaegertracing/jaeger/cmd/builder"
	"github.com/jaegertracing/jaeger/cmd/flags"
	casCfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/storage/spanstore/memory"
)

func newStorageBuilder() *StorageBuilder {
	return &StorageBuilder{
		logger:         zap.NewNop(),
		metricsFactory: metrics.NullFactory,
	}
}

func TestNewCassandraSuccess(t *testing.T) {
	v, _ := config.Viperize(flags.AddFlags)
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(
		sFlags.SpanStorage.Type,
		sFlags.DependencyStorage.DataFrequency,
		basicB.Options.LoggerOption(zap.NewNop()),
		basicB.Options.MetricsFactoryOption(metrics.NullFactory),
		basicB.Options.CassandraSessionOption(&mockSessionBuilder{}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewCassandraFailure(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=sneh"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.EqualError(t, err, "Storage Type is not supported")
	assert.Nil(t, sBuilder)

	command.ParseFlags([]string{"test", "--span-storage.type=cassandra"})
	sFlags.InitFromViper(v)
	sBuilder, err = NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.EqualError(t, err, "Cassandra not configured")
	assert.Nil(t, sBuilder)
}

func TestNewCassandraFailureNoSession(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	command.ParseFlags([]string{"test", "--span-storage.type=cassandra"})
	sFlags.InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency, basicB.Options.CassandraSessionOption(&casCfg.Configuration{}))
	require.Error(t, err)
	assert.Nil(t, sBuilder)
}

func TestNewMemorySuccess(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=memory"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency, basicB.Options.MemoryStoreOption(memory.NewStore()))
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewMemoryFailure(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=memory"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.Error(t, err)
	assert.Nil(t, sBuilder)
}

func TestNewElasticSuccess(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(
		sFlags.SpanStorage.Type,
		sFlags.DependencyStorage.DataFrequency,
		basicB.Options.LoggerOption(zap.NewNop()),
		basicB.Options.ElasticClientOption(&mockEsBuilder{}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, sBuilder)
}

func TestNewElasticFailure(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	sBuilder, err := NewStorageBuilder(sFlags.SpanStorage.Type, sFlags.DependencyStorage.DataFrequency)
	assert.EqualError(t, err, "ElasticSearch not configured")
	assert.Nil(t, sBuilder)
}
