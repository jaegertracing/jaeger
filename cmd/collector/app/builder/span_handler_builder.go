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
	"errors"
	"os"

	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	basicB "github.com/uber/jaeger/cmd/builder"
	"github.com/uber/jaeger/cmd/collector/app"
	zs "github.com/uber/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	"github.com/uber/jaeger/pkg/es"
	escfg "github.com/uber/jaeger/pkg/es/config"
	casSpanstore "github.com/uber/jaeger/plugin/storage/cassandra/spanstore"
	esSpanstore "github.com/uber/jaeger/plugin/storage/es/spanstore"
	"github.com/uber/jaeger/storage/spanstore"
	"github.com/uber/jaeger/storage/spanstore/memory"
)

var (
	errMissingCassandraConfig     = errors.New("Cassandra not configured")
	errMissingMemoryStore         = errors.New("MemoryStore is not provided")
	errMissingElasticSearchConfig = errors.New("ElasticSearch not configured")
)

// SpanHandlerBuilder builds span (Jaeger and zipkin) handlers
type SpanHandlerBuilder interface {
	BuildHandlers() (app.ZipkinSpansHandler, app.JaegerBatchesHandler, error)
}

// NewSpanHandlerBuilder returns a span handler
func NewSpanHandlerBuilder(opts ...basicB.Option) (SpanHandlerBuilder, error) {
	options := basicB.ApplyOptions(opts...)
	if flags.SpanStorage.Type == flags.CassandraStorageType {
		if options.Cassandra == nil {
			return nil, errMissingCassandraConfig
		}
		return newCassandraBuilder(options.Cassandra, options.Logger, options.MetricsFactory), nil
	} else if flags.SpanStorage.Type == flags.MemoryStorageType {
		if options.MemoryStore == nil {
			return nil, errMissingMemoryStore
		}
		return newMemoryStoreBuilder(options.MemoryStore, options.Logger, options.MetricsFactory), nil
	} else if flags.SpanStorage.Type == flags.ESStorageType {
		if options.ElasticSearch == nil {
			return nil, errMissingElasticSearchConfig
		}
		return newESBuilder(options.ElasticSearch, options.Logger), nil
	}
	return nil, flags.ErrUnsupportedStorageType
}

type memoryStoreBuilder struct {
	logger         *zap.Logger
	metricsFactory metrics.Factory
	memStore       *memory.Store
}

func newMemoryStoreBuilder(memStore *memory.Store, logger *zap.Logger, metricsFactory metrics.Factory) *memoryStoreBuilder {
	return &memoryStoreBuilder{
		logger:         logger,
		metricsFactory: metricsFactory,
		memStore:       memStore,
	}
}

func (m *memoryStoreBuilder) BuildHandlers() (app.ZipkinSpansHandler, app.JaegerBatchesHandler, error) {
	return buildHandlers(m.memStore, m.logger, m.metricsFactory)
}

type cassandraSpanHandlerBuilder struct {
	logger         *zap.Logger
	metricsFactory metrics.Factory
	configuration  cascfg.Configuration
	session        cassandra.Session
}

func newCassandraBuilder(config *cascfg.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) *cassandraSpanHandlerBuilder {
	return &cassandraSpanHandlerBuilder{
		logger:         logger,
		metricsFactory: metricsFactory,
		configuration:  fixConfiguration(*config),
	}
}

func (c *cassandraSpanHandlerBuilder) BuildHandlers() (app.ZipkinSpansHandler, app.JaegerBatchesHandler, error) {
	session, err := c.getSession()
	if err != nil {
		return nil, nil, err
	}
	spanStore := casSpanstore.NewSpanWriter(
		session,
		*WriteCacheTTL,
		c.metricsFactory,
		c.logger,
	)

	return buildHandlers(spanStore, c.logger, c.metricsFactory)
}

func defaultSpanFilter(*model.Span) bool {
	return true
}

func (c *cassandraSpanHandlerBuilder) getSession() (cassandra.Session, error) {
	if c.session == nil {
		session, err := c.configuration.NewSession()
		c.session = session
		return c.session, err
	}
	return c.session, nil
}

func fixConfiguration(config cascfg.Configuration) cascfg.Configuration {
	config.ProtoVersion = 4
	return config
}

type esSpanHandlerBuilder struct {
	logger        *zap.Logger
	client        es.Client
	configuration escfg.Configuration
}

func newESBuilder(config *escfg.Configuration, logger *zap.Logger) *esSpanHandlerBuilder {
	return &esSpanHandlerBuilder{
		configuration: *config,
		logger:        logger,
	}
}

func (e *esSpanHandlerBuilder) BuildHandlers() (app.ZipkinSpansHandler, app.JaegerBatchesHandler, error) {
	client, err := e.getClient()
	if err != nil {
		return nil, nil, err
	}
	spanStore := esSpanstore.NewSpanWriter(client, e.logger)

	return buildHandlers(spanStore, e.logger, nil)
}

func buildHandlers(
	spanStore spanstore.Writer,
	logger *zap.Logger,
	metricsFactory metrics.Factory,
) (app.ZipkinSpansHandler, app.JaegerBatchesHandler, error) {
	// TODO: currently ES has no metricsFactory, hence the workaround. Once ES adds a metricsFactory, this nullpointer check should be unnecessary.
	var hostMetrics metrics.Factory
	if metricsFactory != nil {
		hostname, _ := os.Hostname()
		hostMetrics = metricsFactory.Namespace(hostname, nil)
	}

	zSanitizer := zs.NewChainedSanitizer(
		zs.NewSpanDurationSanitizer(logger),
		zs.NewParentIDSanitizer(logger),
	)

	spanProcessor := app.NewSpanProcessor(
		spanStore,
		app.Options.ServiceMetrics(metricsFactory),
		app.Options.HostMetrics(hostMetrics),
		app.Options.Logger(logger),
		app.Options.SpanFilter(defaultSpanFilter),
		app.Options.NumWorkers(*NumWorkers),
		app.Options.QueueSize(*QueueSize),
	)

	return app.NewZipkinSpanHandler(logger, spanProcessor, zSanitizer),
		app.NewJaegerSpanHandler(logger, spanProcessor),
		nil
}

func (e *esSpanHandlerBuilder) getClient() (es.Client, error) {
	if e.client == nil {
		client, err := e.configuration.NewClient()
		e.client = client
		return e.client, err
	}
	return e.client, nil
}
