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

	"github.com/open-telemetry/opentelemetry-collector/component"
	"github.com/open-telemetry/opentelemetry-collector/component/componenterror"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer/consumerdata"
	"github.com/open-telemetry/opentelemetry-collector/consumer/consumererror"
	"github.com/open-telemetry/opentelemetry-collector/exporter/exporterhelper"
	jaegertranslator "github.com/open-telemetry/opentelemetry-collector/translator/trace/jaeger"

	jaegerstorage "github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// NewSpanWriterExporter returns component.TraceExporter
func NewSpanWriterExporter(config configmodels.Exporter, factory jaegerstorage.Factory) (component.TraceExporterOld, error) {
	spanWriter, err := factory.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	storage := storage{Writer: spanWriter}
	return exporterhelper.NewTraceExporterOld(
		config,
		storage.traceDataPusher,
		exporterhelper.WithShutdown(func(context.Context) error {
			if closer, ok := spanWriter.(io.Closer); ok {
				return closer.Close()
			}
			return nil
		}))
}

type storage struct {
	Writer spanstore.Writer
}

// traceDataPusher implements OTEL exporterhelper.traceDataPusher
func (s *storage) traceDataPusher(ctx context.Context, td consumerdata.TraceData) (droppedSpans int, err error) {
	protoBatch, err := jaegertranslator.OCProtoToJaegerProto(td)
	if err != nil {
		return len(td.Spans), consumererror.Permanent(err)
	}
	dropped := 0
	var errs []error
	for _, span := range protoBatch.Spans {
		span.Process = protoBatch.Process
		err := s.Writer.WriteSpan(span)
		if err != nil {
			errs = append(errs, err)
			dropped++
		}
	}
	return dropped, componenterror.CombineErrors(errs)
}
