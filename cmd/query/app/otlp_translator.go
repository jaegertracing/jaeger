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

func OTLP2model(OTLPSpans []byte) ([]*model.Batch, error) {
	ptraceUnmarshaler := ptrace.JSONUnmarshaler{}
	otlpTraces, err := ptraceUnmarshaler.UnmarshalTraces(OTLPSpans)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal OTLP : %w", err)
	}
	jaegerBatches, err := model2otel.ProtoFromTraces(otlpTraces)
	if err != nil {
		return nil, fmt.Errorf("cannot transform OTLP to Jaeger: %w", err)
	}

	return jaegerBatches, nil
}

func BatchesToTraces(jaegerBatches []*model.Batch) ([]model.Trace, error) {
	var jaegerTraces []model.Trace
	spanMap := make(map[model.TraceID][]*model.Span)
	for _, v := range jaegerBatches {
		DenormalizeProcess(v)
		FlattenToSpansMaps(v, spanMap)
	}
	for _, v := range spanMap {
		jaegerTrace := model.Trace{
			Spans: v,
		}
		jaegerTraces = append(jaegerTraces, jaegerTrace)
	}
	return jaegerTraces, nil
}

func DenormalizeProcess(m *model.Batch) {
	for _, v := range m.Spans {
		v.Process = m.Process
	}
}

func FlattenToSpansMaps(m *model.Batch, spanMap map[model.TraceID][]*model.Span) {
	for _, v := range m.Spans {
		val, ok := spanMap[v.TraceID]
		if !ok {
			spanMap[v.TraceID] = []*model.Span{v}
		} else {
			spanMap[v.TraceID] = append(val, v)
		}
	}
}
