// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
	otel2model "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
)

// PTracesSeq2ToModel consumes an otel trace iterator and returns a jaeger model trace.
//
// When necessary, it groups chunks of traces into one single trace
func PTracesSeq2ToModel(seqTrace iter.Seq2[[]ptrace.Traces, error]) ([]*model.Trace, error) {
	var (
		jaegerTraces []*model.Trace
		err error
		tracesByID map[pcommon.TraceID]*model.Trace
	)

	seqTrace(func(otelTraces []ptrace.Traces, e error) bool {
		if e != nil {
			err = e
			return false
		}

		for _, otelTrace := range otelTraces {
			spans := modelSpansFromOtelTrace(otelTrace)
			for _, span := range spans {
				traceId := span.TraceID.ToOTELTraceID()
				if _, exists := tracesByID[traceId]; !exists {
					tracesByID[traceId] = &model.Trace{}
				}
				trace := tracesByID[traceId]
				trace.Spans = append(trace.Spans, span)
				tracesByID[traceId] = trace
			}
		}
		return true
	})

	if err != nil {
		return nil, err
	}

	for _, trace := range tracesByID {
		jaegerTraces = append(jaegerTraces, trace)
	}
	return jaegerTraces, nil
}

// modelSpansFromOtelTrace extracts spans from otel traces
func modelSpansFromOtelTrace(otelTrace ptrace.Traces) []*model.Span {
	spans := []*model.Span{}
	batches := otel2model.ProtoFromTraces(otelTrace)
	for _, batch := range batches {
		spans = append(spans, batch.Spans...)
	}
	return spans
}
