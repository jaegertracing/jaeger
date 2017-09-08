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

	"github.com/uber/jaeger/cmd/builder"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/pkg/cassandra"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/pkg/config"
	"github.com/uber/jaeger/pkg/es"
	escfg "github.com/uber/jaeger/pkg/es/config"
	esMocks "github.com/uber/jaeger/pkg/es/mocks"
	"github.com/uber/jaeger/storage/spanstore/memory"
)

type mockSessionBuilder struct {
}

func (*mockSessionBuilder) NewSession() (cassandra.Session, error) {
	return &mocks.Session{}, nil
}

type mockEsBuilder struct {
	escfg.Configuration
}

func (mck *mockEsBuilder) NewClient() (es.Client, error) {
	return &esMocks.Client{}, nil
}

func TestNewSpanHandlerBuilder(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)

	command.ParseFlags([]string{})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(
		cOpts,
		sFlags,
		builder.Options.LoggerOption(zap.NewNop()),
		builder.Options.MetricsFactoryOption(metrics.NullFactory),
		builder.Options.CassandraSessionOption(&mockSessionBuilder{}),
	)
	require.NoError(t, err)
	assert.NotNil(t, handler)
	zipkin, jaeger := handler.BuildHandlers()
	assert.NotNil(t, zipkin)
	assert.NotNil(t, jaeger)
}

func TestNewSpanHandlerBuilderCassandraNoSession(t *testing.T) {
	v, command := config.Viperize(flags.AddFlags)

	command.ParseFlags([]string{})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(
		cOpts,
		sFlags,
		builder.Options.LoggerOption(zap.NewNop()),
		builder.Options.MetricsFactoryOption(metrics.NullFactory),
		builder.Options.CassandraSessionOption(&cascfg.Configuration{}),
	)
	require.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderCassandraNotConfigured(t *testing.T) {
	v, _ := config.Viperize(AddFlags, flags.AddFlags)
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(cOpts, sFlags)
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderBadStorageTypeFailure(t *testing.T) {
	v, command := config.Viperize(AddFlags, flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=sneh"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(cOpts, sFlags)
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderMemoryNotSet(t *testing.T) {
	v, command := config.Viperize(AddFlags, flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=memory"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(cOpts, sFlags)
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderMemorySet(t *testing.T) {
	v, command := config.Viperize(AddFlags, flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=memory"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(cOpts, sFlags, builder.Options.MemoryStoreOption(memory.NewStore()))
	require.NoError(t, err)
	assert.NotNil(t, handler)
	jHandler, zHandler := handler.BuildHandlers()
	assert.NotNil(t, jHandler)
	assert.NotNil(t, zHandler)
}

func TestNewSpanHandlerBuilderElasticSearch(t *testing.T) {
	v, command := config.Viperize(AddFlags, flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(
		cOpts,
		sFlags,
		builder.Options.LoggerOption(zap.NewNop()),
		builder.Options.ElasticClientOption(&mockEsBuilder{}),
	)
	require.NoError(t, err)
	assert.NotNil(t, handler)
	zipkin, jaeger := handler.BuildHandlers()
	assert.NotNil(t, zipkin)
	assert.NotNil(t, jaeger)
}

func TestNewSpanHandlerBuilderElasticSearchNoClient(t *testing.T) {
	v, command := config.Viperize(AddFlags, flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(
		cOpts,
		sFlags,
		builder.Options.LoggerOption(zap.NewNop()),
		builder.Options.ElasticClientOption(&escfg.Configuration{}),
	)
	require.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderElasticSearchFailure(t *testing.T) {
	v, command := config.Viperize(AddFlags, flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)
	handler, err := NewSpanHandlerBuilder(cOpts, sFlags)
	assert.EqualError(t, err, "ElasticSearch not configured")
	assert.Nil(t, handler)
}

func TestDefaultSpanFilter(t *testing.T) {
	assert.True(t, defaultSpanFilter(nil))
}
