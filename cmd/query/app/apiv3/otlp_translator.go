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
	"fmt"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	model2otel "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
	tracev1 "github.com/jaegertracing/jaeger/proto-gen/otel/trace/v1"
)

func modelToOTLP(spans []*model.Span) ([]*tracev1.ResourceSpans, error) {
	batch := &model.Batch{Spans: spans}
	td, err := model2otel.ProtoToTraces([]*model.Batch{batch})
	if err != nil {
		return nil, fmt.Errorf("cannot convert trace to OpenTelemetry: %w", err)
	}
	req := ptraceotlp.NewExportRequestFromTraces(td)
	// OTEL Collector hides the internal proto implementation, so do a roundtrip conversion (inefficient)
	b, err := req.MarshalProto()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal OTLP: %w", err)
	}
	// use api_v3.SpansResponseChunk which has the same shape as otlp.ExportTraceServiceRequest
	var chunk api_v3.SpansResponseChunk
	if err := proto.Unmarshal(b, &chunk); err != nil {
		return nil, fmt.Errorf("cannot marshal OTLP: %w", err)
	}
	return chunk.ResourceSpans, nil
}

func OTLP2model(OTLPSpans []byte) ([]model.Trace, error) {
	ptrace_unmarshaler := ptrace.JSONUnmarshaler{}
	otlp_traces, err := ptrace_unmarshaler.UnmarshalTraces(OTLPSpans)
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("cannot marshal OTLP : %w", err)
	}
	batches, err := model2otel.ProtoFromTraces(otlp_traces)
   fmt.Println(otlp_traces.ResourceSpans())
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("cannot marshal OTLP : %w", err)
	}
   jaeger_traces := make([]model.Trace, len(batches) )
	for _, v := range batches {
		mar := jsonpb.Marshaler{}
		fmt.Println(mar.MarshalToString(v))
      jaeger_trace := v.ConvertToTraces()
      jaeger_traces = append(jaeger_traces, *jaeger_trace)
	}
	return jaeger_traces, nil
}
