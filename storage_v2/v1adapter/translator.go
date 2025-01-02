// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	jaegerTranslator "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
)

// V1BatchesFromTraces converts OpenTelemetry traces (ptrace.Traces)
// to Jaeger model batches ([]*model.Batch).
func V1BatchesFromTraces(traces ptrace.Traces) []*model.Batch {
	batches := jaegerTranslator.ProtoFromTraces(traces)
	spanMap := createSpanMapFromBatches(batches)
	transferWarningsToModelSpans(traces, spanMap)
	return batches
}

// ProtoFromTraces converts OpenTelemetry traces (ptrace.Traces)
// to Jaeger model batches ([]*model.Batch).
//
// TODO remove this function in favor of V1BatchesFromTraces
func ProtoFromTraces(traces ptrace.Traces) []*model.Batch {
	return V1BatchesFromTraces(traces)
}

// V1BatchesToTraces converts Jaeger model batches ([]*model.Batch)
// to OpenTelemetry traces (ptrace.Traces).
func V1BatchesToTraces(batches []*model.Batch) ptrace.Traces {
	traces, _ := jaegerTranslator.ProtoToTraces(batches) // never returns an error
	spanMap := jptrace.SpanMap(traces, func(s ptrace.Span) pcommon.SpanID {
		return s.SpanID()
	})
	transferWarningsToOTLPSpans(batches, spanMap)
	return traces
}

// V1TracesFromSeq2 converts an interator of ptrace.Traces chunks into v1 traces.
func V1TracesFromSeq2(otelSeq iter.Seq2[[]ptrace.Traces, error]) ([]*model.Trace, error) {
	var (
		jaegerTraces []*model.Trace
		iterErr      error
	)
	jptrace.AggregateTraces(otelSeq)(func(otelTrace ptrace.Traces, err error) bool {
		if err != nil {
			iterErr = err
			return false
		}
		jaegerTraces = append(jaegerTraces, modelTraceFromOtelTrace(otelTrace))
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	return jaegerTraces, nil
}

// V1TraceToOtelTrace converts v1 traces (*model.Trace) to Otel traces (ptrace.Traces)
func V1TraceToOtelTrace(jTrace *model.Trace) ptrace.Traces {
	batches := createBatchesFromModelTrace(jTrace)
	return V1BatchesToTraces(batches)
}

func createBatchesFromModelTrace(jTrace *model.Trace) []*model.Batch {
	spans := jTrace.Spans

	if len(spans) == 0 {
		return nil
	}
	batch := &model.Batch{
		Spans: jTrace.Spans,
	}
	return []*model.Batch{batch}
}

// modelTraceFromOtelTrace extracts spans from otel traces
func modelTraceFromOtelTrace(otelTrace ptrace.Traces) *model.Trace {
	var spans []*model.Span
	batches := V1BatchesFromTraces(otelTrace)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				proc := *batch.Process // shallow clone
				span.Process = &proc
			}
			spans = append(spans, span)
		}
	}
	return &model.Trace{Spans: spans}
}

func createSpanMapFromBatches(batches []*model.Batch) map[model.SpanID]*model.Span {
	spanMap := make(map[model.SpanID]*model.Span)
	for _, batch := range batches {
		for _, span := range batch.Spans {
			spanMap[span.SpanID] = span
		}
	}
	return spanMap
}

func transferWarningsToModelSpans(traces ptrace.Traces, spanMap map[model.SpanID]*model.Span) {
	resources := traces.ResourceSpans()
	for i := 0; i < resources.Len(); i++ {
		scopes := resources.At(i).ScopeSpans()
		for j := 0; j < scopes.Len(); j++ {
			spans := scopes.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				otelSpan := spans.At(k)
				warnings := jptrace.GetWarnings(otelSpan)
				if len(warnings) == 0 {
					continue
				}
				if span, ok := spanMap[model.SpanIDFromOTEL(otelSpan.SpanID())]; ok {
					span.Warnings = append(span.Warnings, warnings...)
					// filter out the warning tag
					span.Tags = filterTags(span.Tags, jptrace.WarningsAttribute)
				}
			}
		}
	}
}

func transferWarningsToOTLPSpans(batches []*model.Batch, spanMap map[pcommon.SpanID]ptrace.Span) {
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if len(span.Warnings) == 0 {
				continue
			}
			if otelSpan, ok := spanMap[span.SpanID.ToOTELSpanID()]; ok {
				jptrace.AddWarnings(otelSpan, span.Warnings...)
			}
		}
	}
}

func filterTags(tags []model.KeyValue, keyToRemove string) []model.KeyValue {
	var filteredTags []model.KeyValue
	for _, tag := range tags {
		if tag.Key != keyToRemove {
			filteredTags = append(filteredTags, tag)
		}
	}
	return filteredTags
}
