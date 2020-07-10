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
	"io"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenterror"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	jaegertranslator "go.opentelemetry.io/collector/translator/trace/jaeger"

	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// NewSpanWriterExporter returns component.TraceExporter
func NewSpanWriterExporter(config configmodels.Exporter, factory storage.Factory) (component.TraceExporter, error) {
	spanWriter, err := factory.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	storage := store{Writer: spanWriter}
	return exporterhelper.NewTraceExporter(
		config,
		storage.traceDataPusher,
		exporterhelper.WithShutdown(func(context.Context) error {
			if closer, ok := spanWriter.(io.Closer); ok {
				return closer.Close()
			}
			return nil
		}))
}

type store struct {
	Writer spanstore.Writer
}

// traceDataPusher implements OTEL exporterhelper.traceDataPusher
func (s *store) traceDataPusher(ctx context.Context, td pdata.Traces) (droppedSpans int, err error) {
	batches, err := jaegertranslator.InternalTracesToJaegerProto(td)
	if err != nil {
		return td.SpanCount(), consumererror.Permanent(err)
	}
	dropped := 0
	var errs []error
	for _, batch := range batches {
		for _, span := range batch.Spans {
			span.Process = batch.Process
			err := s.Writer.WriteSpan(span)
			if err != nil {
				errs = append(errs, err)
				dropped++
			}
		}
	}
	return dropped, componenterror.CombineErrors(errs)
}
