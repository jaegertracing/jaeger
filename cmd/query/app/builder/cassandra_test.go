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
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/pkg/cassandra/config"
)

func withBuilder(t *testing.T, f func(builder *cassandraBuilder)) {
	sFlags := &flags.SharedFlags{}
	cBuilder, err := newCassandraBuilder(&mockSessionBuilder{}, zap.NewNop(), metrics.NullFactory, sFlags.DependencyStorage.DataFrequency)
	require.NoError(t, err)

	f(cBuilder)
}

func TestNewCassandraBuildFailure(t *testing.T) {
	cfg := &config.Configuration{
		Servers: []string{"127.0.0.1"},
	}
	sFlags := &flags.SharedFlags{}
	cBuilder, err := newCassandraBuilder(cfg, zap.NewNop(), metrics.NullFactory, sFlags.DependencyStorage.DataFrequency)
	require.Error(t, err)
	assert.Nil(t, cBuilder)
}

func TestNewCassandraBuild(t *testing.T) {
	withBuilder(t, func(cBuilder *cassandraBuilder) {
		assert.NotNil(t, cBuilder)
		assert.NotNil(t, cBuilder.metricsFactory)
		assert.NotNil(t, cBuilder.logger)
		assert.NotNil(t, cBuilder.session)
	})
}

func TestNewReaderSuccesses(t *testing.T) {
	withBuilder(t, func(cBuilder *cassandraBuilder) {
		spanReader, err := cBuilder.NewSpanReader()
		assert.NoError(t, err)
		assert.NotNil(t, spanReader)
		depReader, err := cBuilder.NewDependencyReader()
		assert.NoError(t, err)
		assert.NotNil(t, depReader)
	})
}
