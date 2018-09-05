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

	basicB "github.com/jaegertracing/jaeger/cmd/builder"
	"github.com/jaegertracing/jaeger/cmd/collector/app"
	zs "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/plugin"
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
	pluginsFactory plugin.PluginsFactory
}

// NewSpanHandlerBuilder returns new SpanHandlerBuilder with configured span storage.
func NewSpanHandlerBuilder(cOpts *CollectorOptions, spanWriter spanstore.Writer, opts ...basicB.Option) (*SpanHandlerBuilder, error) {
	options := basicB.ApplyOptions(opts...)

	pluginsFactory := plugin.NewPluginsFactory(cOpts.CollectorPluginsDir, options.Logger)
	err := pluginsFactory.Load()
	if err != nil {
		return nil, err
	}

	spanHb := &SpanHandlerBuilder{
		collectorOpts:  cOpts,
		logger:         options.Logger,
		metricsFactory: options.MetricsFactory,
		spanWriter:     spanWriter,
		pluginsFactory:	pluginsFactory,
	}

	return spanHb, nil
}

// BuildHandlers builds span handlers (Zipkin, Jaeger)
func (spanHb *SpanHandlerBuilder) BuildHandlers() (
	app.ZipkinSpansHandler,
	app.JaegerBatchesHandler,
	*app.GRPCHandler,
) {
	hostname, _ := os.Hostname()
	hostMetrics := spanHb.metricsFactory.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"host": hostname}})

	spanProcessor := app.NewSpanProcessor(
		spanHb.spanWriter,
		app.Options.ServiceMetrics(spanHb.metricsFactory),
		app.Options.HostMetrics(hostMetrics),
		app.Options.Logger(spanHb.logger),
		app.Options.SpanFilter(defaultSpanFilter),
		app.Options.NumWorkers(spanHb.collectorOpts.NumWorkers),
		app.Options.QueueSize(spanHb.collectorOpts.QueueSize),
		app.Options.PreProcessSpans(spanHb.pluginsFactory.PreProcessor()),
		app.Options.SpanFilter(spanHb.pluginsFactory.SpanFilter()),
		app.Options.Sanitizer(spanHb.pluginsFactory.Sanitizer()),
		app.Options.PreSave(spanHb.pluginsFactory.PreSaveProcessor()),
	)

	return app.NewZipkinSpanHandler(spanHb.logger, spanProcessor, zs.NewChainedSanitizer(zs.StandardSanitizers...)),
		app.NewJaegerSpanHandler(spanHb.logger, spanProcessor),
		app.NewGRPCHandler(spanHb.logger, spanProcessor)
}

func defaultSpanFilter(*model.Span) bool {
	return true
}
