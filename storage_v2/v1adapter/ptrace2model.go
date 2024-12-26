// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	otel2model "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
)

// PTracesSeq2ToModel consumes tracesSeq.
// When necessary, it groups spans from *consecutive* chunks of ptrace.Traces into a single model.Trace
// It adheres to the chunking requirement of tracestore.Reader.GetTraces.
//
// 
// Returns nil, and spanstore.ErrTraceNotFound for empty iterators
func PTracesSeq2ToModel(tracesSeq iter.Seq2[[]ptrace.Traces, error]) ([]*model.Trace, error) {
	jaegerTraces := []*model.Trace{}
	otelTraces, err := iter.CollectWithErrors(jptrace.AggregateTraces(tracesSeq))
	if err != nil {
		return nil, err
	}
	if len(otelTraces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}

	for _, otelTrace := range otelTraces {
		jTrace := &model.Trace{
			Spans: modelSpansFromOtelTrace(otelTrace),
		}
		jaegerTraces = append(jaegerTraces, jTrace)
	}
	return jaegerTraces, nil
}

// modelSpansFromOtelTrace extracts spans from otel traces
func modelSpansFromOtelTrace(otelTrace ptrace.Traces) []*model.Span {
	spans := []*model.Span{}
	batches := otel2model.ProtoFromTraces(otelTrace)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = &model.Process{ // give each span it's own process, avoid potential side effects from shared Process objects.
					ServiceName: batch.Process.ServiceName,
					Tags:        batch.Process.Tags,
				}
			}
			spans = append(spans, span)
		}
	}
	return spans
}
