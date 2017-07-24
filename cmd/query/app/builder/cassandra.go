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
	configuration           cascfg.Configuration
	session                 cassandra.Session
	dependencyDataFrequency time.Duration
}

func newCassandraBuilder(config *cascfg.Configuration, logger *zap.Logger, metricsFactory metrics.Factory, dependencyDataFreq time.Duration) *cassandraBuilder {
	cBuilder := &cassandraBuilder{
		logger:                  logger,
		metricsFactory:          metricsFactory,
		configuration:           fixConfiguration(*config),
		dependencyDataFrequency: dependencyDataFreq,
	}
	return cBuilder
}

func fixConfiguration(config cascfg.Configuration) cascfg.Configuration {
	config.ProtoVersion = 4
	return config
}

func (c *cassandraBuilder) getSession() (cassandra.Session, error) {
	if c.session == nil {
		session, err := c.configuration.NewSession()
		c.session = session
		return c.session, err
	}
	return c.session, nil
}

func (c *cassandraBuilder) NewSpanReader() (spanstore.Reader, error) {
	session, err := c.getSession()
	if err != nil {
		return nil, err
	}
	return cSpanStore.NewSpanReader(session, c.metricsFactory, c.logger), nil
}

func (c *cassandraBuilder) NewDependencyReader() (dependencystore.Reader, error) {
	session, err := c.getSession()
	if err != nil {
		return nil, err
	}
	return cDependencyStore.NewDependencyStore(session, c.dependencyDataFrequency, c.metricsFactory, c.logger), nil
}
