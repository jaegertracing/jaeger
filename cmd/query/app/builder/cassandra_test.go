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
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
)

func withBuilder(f func(builder *cassandraBuilder)) {
	cfg := &config.Configuration{
		Servers: []string{"127.0.0.1"},
	}
	cBuilder := newCassandraBuilder(cfg, zap.NewNop(), metrics.NullFactory)
	f(cBuilder)
}

func TestNewCassandraBuild(t *testing.T) {
	withBuilder(func(cBuilder *cassandraBuilder) {
		assert.NotNil(t, cBuilder)
		assert.NotNil(t, cBuilder.metricsFactory)
		assert.NotNil(t, cBuilder.logger)
		assert.NotNil(t, cBuilder.configuration)
		assert.Nil(t, cBuilder.session)
	})
}

func TestNewReaderFailures(t *testing.T) {
	withBuilder(func(cBuilder *cassandraBuilder) {
		tableName := "dependencies"
		cBuilder.configuration.Servers = []string{"invalidhostname"}
		spanReader, err := cBuilder.NewSpanReader()
		assert.Error(t, err)
		assert.Nil(t, spanReader)
		depReader, err := cBuilder.NewDependencyReader(tableName)
		assert.Error(t, err)
		assert.Nil(t, depReader)
	})
}

func TestNewReaderSuccesses(t *testing.T) {
	withBuilder(func(cBuilder *cassandraBuilder) {
		tableName := "dependencies"
		mockSession := mocks.Session{}
		cBuilder.session = &mockSession
		spanReader, err := cBuilder.NewSpanReader()
		assert.NoError(t, err)
		assert.NotNil(t, spanReader)
		depReader, err := cBuilder.NewDependencyReader(tableName)
		assert.NoError(t, err)
		assert.NotNil(t, depReader)
	})
}
