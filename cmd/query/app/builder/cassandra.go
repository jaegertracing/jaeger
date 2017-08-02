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
	"time"

	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/pkg/cassandra"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	cDependencyStore "github.com/uber/jaeger/plugin/storage/cassandra/dependencystore"
	cSpanStore "github.com/uber/jaeger/plugin/storage/cassandra/spanstore"
	"github.com/uber/jaeger/storage/dependencystore"
	"github.com/uber/jaeger/storage/spanstore"
)

type cassandraBuilder struct {
	logger                  *zap.Logger
	metricsFactory          metrics.Factory
	session                 cassandra.Session
	dependencyDataFrequency time.Duration
}

func newCassandraBuilder(config cascfg.SessionBuilder, logger *zap.Logger, metricsFactory metrics.Factory, dependencyDataFreq time.Duration) (*cassandraBuilder, error) {
	session, err := config.NewSession()
	if err != nil {
		return nil, err
	}

	cBuilder := &cassandraBuilder{
		session:                 session,
		logger:                  logger,
		metricsFactory:          metricsFactory,
		dependencyDataFrequency: dependencyDataFreq,
	}
	return cBuilder, nil
}

func (c *cassandraBuilder) NewSpanReader() (spanstore.Reader, error) {
	return cSpanStore.NewSpanReader(c.session, c.metricsFactory, c.logger), nil
}

func (c *cassandraBuilder) NewDependencyReader() (dependencystore.Reader, error) {
	return cDependencyStore.NewDependencyStore(c.session, c.dependencyDataFrequency, c.metricsFactory, c.logger), nil
}
