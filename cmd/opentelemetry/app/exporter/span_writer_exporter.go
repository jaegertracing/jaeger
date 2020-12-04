// Copyright (c) 2020 The Jaeger Authors.
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

package exporter

import (
	"context"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenterror"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	jaegertranslator "go.opentelemetry.io/collector/translator/trace/jaeger"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/storagemetrics"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// NewSpanWriterExporter returns component.TraceExporter
func NewSpanWriterExporter(config configmodels.Exporter, params component.ExporterCreateParams, factory storage.Factory, opts ...exporterhelper.Option) (component.TracesExporter, error) {
	spanWriter, err := factory.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	storage := store{Writer: spanWriter, storageNameTag: tag.Insert(storagemetrics.TagExporterName(), config.Name())}
	return exporterhelper.NewTraceExporter(
		config,
		params.Logger,
		storage.traceDataPusher,
		opts...)
}

type store struct {
	Writer         spanstore.Writer
	storageNameTag tag.Mutator
}

// traceDataPusher implements OTEL exporterhelper.traceDataPusher
func (s *store) traceDataPusher(ctx context.Context, td pdata.Traces) (droppedSpans int, err error) {
	batches, err := jaegertranslator.InternalTracesToJaegerProto(td)
	if err != nil {
		return td.SpanCount(), consumererror.Permanent(err)
	}
	dropped := 0
	var errs []error
	storedSpans := map[string]int64{}
	notStoredSpans := map[string]int64{}
	for _, batch := range batches {
		for _, span := range batch.Spans {
			span.Process = batch.Process
			err := s.Writer.WriteSpan(ctx, span)
			if err != nil {
				errs = append(errs, err)
				dropped++
				notStoredSpans[span.Process.ServiceName] = notStoredSpans[span.Process.ServiceName] + 1
			} else {
				storedSpans[span.Process.ServiceName] = storedSpans[span.Process.ServiceName] + 1
			}
		}
	}
	for k, v := range notStoredSpans {
		ctx, _ := tag.New(ctx,
			tag.Insert(storagemetrics.TagServiceName(), k), s.storageNameTag)
		stats.Record(ctx, storagemetrics.StatSpansNotStoredCount().M(v))
	}
	for k, v := range storedSpans {
		ctx, _ := tag.New(ctx,
			tag.Insert(storagemetrics.TagServiceName(), k), s.storageNameTag)
		stats.Record(ctx, storagemetrics.StatSpansStoredCount().M(v))
	}
	return dropped, componenterror.CombineErrors(errs)
}
