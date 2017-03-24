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
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/cmd/builder"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/storage/spanstore/memory"
)

func TestNewSpanHandlerBuilder(t *testing.T) {
	flag.Parse()
	handler, err := NewSpanHandlerBuilder(
		builder.Options.LoggerOption(zap.New(zap.NullEncoder())),
		builder.Options.MetricsFactoryOption(metrics.NullFactory),
		builder.Options.CassandraOption(&cascfg.Configuration{
			Servers: []string{"127.0.0.1"},
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, handler)
}

func TestNewSpanHandlerBuilderCassandraNotConfigured(t *testing.T) {
	flag.Parse()
	handler, err := NewSpanHandlerBuilder()
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderBadStorageTypeFailure(t *testing.T) {
	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"test", "--span-storage.type=sneh"}
	flag.Parse()
	handler, err := NewSpanHandlerBuilder()
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderMemoryNotSet(t *testing.T) {
	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"test", "--span-storage.type=memory"}
	flag.Parse()
	handler, err := NewSpanHandlerBuilder()
	assert.Error(t, err)
	assert.Nil(t, handler)
}

func TestNewSpanHandlerBuilderMemorySet(t *testing.T) {
	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = []string{"test", "--span-storage.type=memory"}
	flag.Parse()
	handler, err := NewSpanHandlerBuilder(builder.Options.MemoryStoreOption(memory.NewStore()))
	assert.NoError(t, err)
	assert.NotNil(t, handler)
	jHandler, zHandler, err := handler.BuildHandlers()
	assert.NoError(t, err)
	assert.NotNil(t, jHandler)
	assert.NotNil(t, zHandler)
}

func withCassandraBuilder(f func(builder *cassandraSpanHandlerBuilder)) {
	cfg := &cascfg.Configuration{
		Servers: []string{"127.0.0.1"},
	}
	cBuilder := newCassandraBuilder(cfg, zap.New(zap.NullEncoder()), metrics.NullFactory)
	f(cBuilder)
}

func TestBuildHandlers(t *testing.T) {
	withCassandraBuilder(func(cBuilder *cassandraSpanHandlerBuilder) {
		mockSession := mocks.Session{}
		cBuilder.session = &mockSession
		zHandler, jHandler, err := cBuilder.BuildHandlers()
		assert.NoError(t, err)
		assert.NotNil(t, zHandler)
		assert.NotNil(t, jHandler)
	})
}

func TestBuildHandlersFailure(t *testing.T) {
	withCassandraBuilder(func(cBuilder *cassandraSpanHandlerBuilder) {
		cBuilder.configuration.Servers = []string{"badhostname"}
		zHandler, jHandler, err := cBuilder.BuildHandlers()
		assert.Error(t, err)
		assert.Nil(t, zHandler)
		assert.Nil(t, jHandler)
	})
}

func TestDefaultSpanFilter(t *testing.T) {
	assert.True(t, defaultSpanFilter(nil))
}
