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
	"testing"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
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
		builder.Options.CassandraOption(&cascfg.Configuration{
			Servers: []string{"127.0.0.1"},
		}),
	)
	assert.Error(t, err)
	assert.Equal(t, gocql.ErrNoConnectionsStarted, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderCassandraNotConfigured(t *testing.T) {
	v, _ := config.Viperize(AddFlags, flags.AddFlags)
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	handler, err := NewSpanHandlerBuilder(cOpts, sFlags)
	assert.Error(t, err)
	assert.Equal(t, "Cassandra not configured", err.Error())
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
	assert.NoError(t, err)
	assert.NotNil(t, handler)
	jHandler, zHandler, err := handler.BuildHandlers()
	require.NoError(t, err)
	assert.NotNil(t, jHandler)
	assert.NotNil(t, zHandler)
}

func TestNewSpanHandlerBuilderElasticSearch(t *testing.T) {
	v, command := config.Viperize(AddFlags, flags.AddFlags)
	command.ParseFlags([]string{"test", "--span-storage.type=elasticsearch"})
	sFlags := new(flags.SharedFlags).InitFromViper(v)
	cOpts := new(CollectorOptions).InitFromViper(v)

	_, err := NewSpanHandlerBuilder(
		cOpts,
		sFlags,
		builder.Options.LoggerOption(zap.NewNop()),
		builder.Options.ElasticSearchOption(&escfg.Configuration{}),
	)
	assert.Error(t, err)
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

func withBuilder(f func(builder *SpanHandlerBuilder)) {
	cOpts := &CollectorOptions{}
	spanBuilder := &SpanHandlerBuilder{
		logger:         zap.NewNop(),
		collectorOpts:  cOpts,
		metricsFactory: metrics.NullFactory,
	}

	f(spanBuilder)
}

type mockSessionBuilder struct {
}

func (at *mockSessionBuilder) NewSession() (cassandra.Session, error) {
	return &mocks.Session{}, nil
}

func TestBuildHandlersCassandra(t *testing.T) {
	withBuilder(func(builder *SpanHandlerBuilder) {
		var err error
		builder.spanWriter, err = builder.initCassStore(new(mockSessionBuilder))
		require.NoError(t, err)

		zHandler, jHandler, err := builder.BuildHandlers()
		require.NoError(t, err)
		assert.NotNil(t, zHandler)
		assert.NotNil(t, jHandler)
	})
}

func TestBuildHandlersCassandraFailure(t *testing.T) {
	withBuilder(func(cBuilder *SpanHandlerBuilder) {
		cfg := &cascfg.Configuration{
			Servers: []string{"badhostname"},
		}
		_, err := cBuilder.initCassStore(cfg)
		assert.Error(t, err)
	})
}

type MockEsBuilder struct {
}

func (mck *MockEsBuilder) NewClient() (es.Client, error) {
	return &esMocks.Client{}, nil
}

func TestBuildHandlersElasticSearch(t *testing.T) {
	withBuilder(func(builder *SpanHandlerBuilder) {
		spanWriter, err := builder.initElasticStore(&MockEsBuilder{})
		require.NoError(t, err)
		require.NotNil(t, spanWriter)

		builder.spanWriter = spanWriter
		zHandler, jHandler, err := builder.BuildHandlers()
		assert.NoError(t, err)
		assert.NotNil(t, zHandler)
		assert.NotNil(t, jHandler)
	})
}

func TestBuildHandlersElasticSearchFailure(t *testing.T) {
	withBuilder(func(builder *SpanHandlerBuilder) {
		spanWriter, err := builder.initElasticStore(&escfg.Configuration{})
		assert.Error(t, err)
		assert.Nil(t, spanWriter)
	})
}
