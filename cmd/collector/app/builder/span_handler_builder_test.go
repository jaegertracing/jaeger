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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"fmt"
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

	fmt.Println(sFlags.SpanStorage.Type)
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
