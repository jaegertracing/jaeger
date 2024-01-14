// Copyright (c) 2021 The Jaeger Authors.
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

package apiv3

import (
	model2otel "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
)

func modelToOTLP(spans []*model.Span) (ptrace.Traces, error) {
	batch := &model.Batch{Spans: spans}
	return model2otel.ProtoToTraces([]*model.Batch{batch})
}

// func Unused_modelToOTLP(spans []*model.Span) ([]*tracev1.ResourceSpans, error) {
// 	batch := &model.Batch{Spans: spans}
// 	td, err := model2otel.ProtoToTraces([]*model.Batch{batch})
// 	if err != nil {
// 		return nil, fmt.Errorf("cannot convert trace to OpenTelemetry: %w", err)
// 	}
// 	req := ptraceotlp.NewExportRequestFromTraces(td)
// 	// OTEL Collector hides the internal proto implementation, so do a roundtrip conversion (inefficient)
// 	b, err := req.MarshalProto()
// 	if err != nil {
// 		return nil, fmt.Errorf("cannot marshal OTLP: %w", err)
// 	}
// 	// use api_v3.SpansResponseChunk which has the same shape as otlp.ExportTraceServiceRequest
// 	var chunk api_v3.SpansResponseChunk
// 	if err := proto.Unmarshal(b, &chunk); err != nil {
// 		return nil, fmt.Errorf("cannot marshal OTLP: %w", err)
// 	}
// 	return chunk.ResourceSpans, nil
// }
