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
	"errors"
	"os"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	basicB "github.com/uber/jaeger/cmd/builder"
	"github.com/uber/jaeger/cmd/collector/app"
	zs "github.com/uber/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/model"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	escfg "github.com/uber/jaeger/pkg/es/config"
	casSpanstore "github.com/uber/jaeger/plugin/storage/cassandra/spanstore"
	esSpanstore "github.com/uber/jaeger/plugin/storage/es/spanstore"
	"github.com/uber/jaeger/storage/spanstore"
)

var (
	errMissingCassandraConfig     = errors.New("Cassandra not configured")
	errMissingMemoryStore         = errors.New("MemoryStore is not provided")
	errMissingElasticSearchConfig = errors.New("ElasticSearch not configured")
)

// SpanHandlerBuilder holds configuration required for handlers
type SpanHandlerBuilder struct {
	logger         *zap.Logger
	metricsFactory metrics.Factory
	collectorOpts  *CollectorOptions
	spanWriter     spanstore.Writer
}

// NewSpanHandlerBuilder returns new SpanHandlerBuilder with configured span storage.
func NewSpanHandlerBuilder(cOpts *CollectorOptions, sFlags *flags.SharedFlags, opts ...basicB.Option) (*SpanHandlerBuilder, error) {
	options := basicB.ApplyOptions(opts...)

	spanHb := &SpanHandlerBuilder{
		collectorOpts:  cOpts,
		logger:         options.Logger,
		metricsFactory: options.MetricsFactory,
	}

	var err error
	if sFlags.SpanStorage.Type == flags.CassandraStorageType {
		if options.CassandraSessionBuilder == nil {
			return nil, errMissingCassandraConfig
		}
		spanHb.spanWriter, err = spanHb.initCassStore(options.CassandraSessionBuilder)
	} else if sFlags.SpanStorage.Type == flags.MemoryStorageType {
		if options.MemoryStore == nil {
			return nil, errMissingMemoryStore
		}
		spanHb.spanWriter = options.MemoryStore
	} else if sFlags.SpanStorage.Type == flags.ESStorageType {
		if options.ElasticClientBuilder == nil {
			return nil, errMissingElasticSearchConfig
		}
		spanHb.spanWriter, err = spanHb.initElasticStore(options.ElasticClientBuilder)
	} else {
		return nil, flags.ErrUnsupportedStorageType
	}

	if err != nil {
		return nil, err
	}

	return spanHb, nil
}

func (spanHb *SpanHandlerBuilder) initCassStore(builder cascfg.SessionBuilder) (spanstore.Writer, error) {
	session, err := builder.NewSession()
	if err != nil {
		return nil, err
	}

	return casSpanstore.NewSpanWriter(
		session,
		spanHb.collectorOpts.WriteCacheTTL,
		spanHb.metricsFactory,
		spanHb.logger,
	), nil
}

func (spanHb *SpanHandlerBuilder) initElasticStore(esBuilder escfg.ClientBuilder) (spanstore.Writer, error) {
	client, err := esBuilder.NewClient()
	if err != nil {
		return nil, err
	}

	return esSpanstore.NewSpanWriter(
		client,
		spanHb.logger,
		spanHb.metricsFactory,
		esBuilder.GetNumShards(),
		esBuilder.GetNumReplicas(),
	), nil
}

// BuildHandlers builds span handlers (Zipkin, Jaeger)
func (spanHb *SpanHandlerBuilder) BuildHandlers() (app.ZipkinSpansHandler, app.JaegerBatchesHandler) {
	hostname, _ := os.Hostname()
	hostMetrics := spanHb.metricsFactory.Namespace(hostname, nil)

	zSanitizer := zs.NewChainedSanitizer(
		zs.NewSpanDurationSanitizer(),
		zs.NewSpanStartTimeSanitizer(),
		zs.NewParentIDSanitizer(),
		zs.NewErrorTagSanitizer(),
	)

	spanProcessor := app.NewSpanProcessor(
		spanHb.spanWriter,
		app.Options.ServiceMetrics(spanHb.metricsFactory),
		app.Options.HostMetrics(hostMetrics),
		app.Options.Logger(spanHb.logger),
		app.Options.SpanFilter(defaultSpanFilter),
		app.Options.NumWorkers(spanHb.collectorOpts.NumWorkers),
		app.Options.QueueSize(spanHb.collectorOpts.QueueSize),
	)

	return app.NewZipkinSpanHandler(spanHb.logger, spanProcessor, zSanitizer),
		app.NewJaegerSpanHandler(spanHb.logger, spanProcessor)
}

func defaultSpanFilter(*model.Span) bool {
	return true
}
