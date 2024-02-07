// Copyright (c) 2024 The Jaeger Authors.
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

package app

import (
	"fmt"

	model2otel "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
)

func otlp2model(otlpSpans []byte) ([]*model.Batch, error) {
	ptraceUnmarshaler := ptrace.JSONUnmarshaler{}
	otlpTraces, err := ptraceUnmarshaler.UnmarshalTraces(otlpSpans)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal OTLP : %w", err)
	}
	jaegerBatches, err := model2otel.ProtoFromTraces(otlpTraces)
	if err != nil {
		return nil, fmt.Errorf("cannot transform OTLP to Jaeger: %w", err)
	}
	return jaegerBatches, nil
}

func batchesToTraces(jaegerBatches []*model.Batch) ([]*model.Trace, error) {
	var traces []*model.Trace
	traceMap := make(map[model.TraceID]*model.Trace)
	for _, batch := range jaegerBatches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
			trace, ok := traceMap[span.TraceID]
			if !ok {
				newtrace := model.Trace{
					Spans: []*model.Span{span},
				}
				traceMap[span.TraceID] = &newtrace
				traces = append(traces, &newtrace)
			} else {
				trace.Spans = append(trace.Spans, span)
			}
		}
	}
	return traces, nil
}
