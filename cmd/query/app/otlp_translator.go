// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/v1adapter"
)

func otlp2traces(otlpSpans []byte) ([]*model.Trace, error) {
	ptraceUnmarshaler := ptrace.JSONUnmarshaler{}
	otlpTraces, err := ptraceUnmarshaler.UnmarshalTraces(otlpSpans)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal OTLP : %w", err)
	}
	jaegerBatches := v1adapter.ProtoFromTraces(otlpTraces)
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
