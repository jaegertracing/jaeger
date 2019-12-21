// Copyright (c) 2019 The Jaeger Authors.
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
	"os"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	zs "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// SpanHandlerBuilder holds configuration required for handlers
type SpanHandlerBuilder struct {
	SpanWriter     spanstore.Writer
	CollectorOpts  CollectorOptions
	Logger         *zap.Logger
	MetricsFactory metrics.Factory
}

// BuildHandlers builds span handlers (Zipkin, Jaeger)
func (spanHb *SpanHandlerBuilder) BuildHandlers() (
	app.ZipkinSpansHandler,
	app.JaegerBatchesHandler,
	*app.GRPCHandler,
) {
	hostname, _ := os.Hostname()
	svcMetrics := spanHb.metricsFactory()
	hostMetrics := svcMetrics.Namespace(metrics.NSOptions{Tags: map[string]string{"host": hostname}})

	spanProcessor := app.NewSpanProcessor(
		spanHb.SpanWriter,
		app.Options.ServiceMetrics(svcMetrics),
		app.Options.HostMetrics(hostMetrics),
		app.Options.Logger(spanHb.logger()),
		app.Options.SpanFilter(defaultSpanFilter),
		app.Options.NumWorkers(spanHb.CollectorOpts.NumWorkers),
		app.Options.QueueSize(spanHb.CollectorOpts.QueueSize),
		app.Options.CollectorTags(spanHb.CollectorOpts.CollectorTags),
	)

	return app.NewZipkinSpanHandler(spanHb.Logger, spanProcessor, zs.NewChainedSanitizer(zs.StandardSanitizers...)),
		app.NewJaegerSpanHandler(spanHb.Logger, spanProcessor),
		app.NewGRPCHandler(spanHb.Logger, spanProcessor)
}

func defaultSpanFilter(*model.Span) bool {
	return true
}

func (b *SpanHandlerBuilder) logger() *zap.Logger {
	if b.Logger == nil {
		return zap.NewNop()
	}
	return b.Logger
}

func (b *SpanHandlerBuilder) metricsFactory() metrics.Factory {
	if b.MetricsFactory == nil {
		return metrics.NullFactory
	}
	return b.MetricsFactory
}
