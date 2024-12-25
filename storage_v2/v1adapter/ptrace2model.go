// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	otel2model "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
)

// PTracesSeq2ToModel consumes an iterator seqTrace. When necessary,
// it groups spans from *consecutive* chunks of ptrace.Traces into a single model.Trace
//
// Returns nil, and spanstore.ErrTraceNotFound for empty iterators
func PTracesSeq2ToModel(seqTrace iter.Seq2[[]ptrace.Traces, error]) ([]*model.Trace, error) {
	var (
		err         error
		lastTraceID *model.TraceID
		lastTrace   *model.Trace
	)
	jaegerTraces := []*model.Trace{}

	seqTrace(func(otelTraces []ptrace.Traces, e error) bool {
		if e != nil {
			err = e
			return false
		}

		for _, otelTrace := range otelTraces {
			spans := modelSpansFromOtelTrace(otelTrace)
			if len(spans) == 0 {
				continue
			}
			currentTraceID := spans[0].TraceID
			if lastTraceID != nil && *lastTraceID == currentTraceID {
				lastTrace.Spans = append(lastTrace.Spans, spans...)
			} else {
				newTrace := &model.Trace{Spans: spans}
				lastTraceID = &currentTraceID
				lastTrace = newTrace
				jaegerTraces = append(jaegerTraces, lastTrace)
			}
		}
		return true
	})

	if err != nil {
		return nil, err
	}

	if len(jaegerTraces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return jaegerTraces, nil
}

// modelSpansFromOtelTrace extracts spans from otel traces
func modelSpansFromOtelTrace(otelTrace ptrace.Traces) []*model.Span {
	spans := []*model.Span{}
	batches := otel2model.ProtoFromTraces(otelTrace)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			span.Process = &model.Process{}
			span.Process.ServiceName = batch.GetProcess().GetServiceName()
			span.Process.Tags = batch.GetProcess().GetTags()
			spans = append(spans, span)
		}
	}
	return spans
}
