// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	jaegerTranslator "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
)

// ProtoFromTraces converts OpenTelemetry traces (ptrace.Traces)
// to Jaeger model batches ([]*model.Batch).
func ProtoFromTraces(traces ptrace.Traces) []*model.Batch {
	batches := jaegerTranslator.ProtoFromTraces(traces)
	spanMap := createSpanMapFromTraces(traces)
	addWarningsToBatches(batches, spanMap)
	return batches
}

func createSpanMapFromTraces(traces ptrace.Traces) map[pcommon.SpanID]ptrace.Span {
	spanMap := make(map[pcommon.SpanID]ptrace.Span)
	resources := traces.ResourceSpans()
	for i := 0; i < resources.Len(); i++ {
		scopes := resources.At(i).ScopeSpans()
		for j := 0; j < scopes.Len(); j++ {
			spans := scopes.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				spanMap[span.SpanID()] = span
			}
		}
	}
	return spanMap
}

func addWarningsToBatches(batches []*model.Batch, spanMap map[pcommon.SpanID]ptrace.Span) {
	for i, batch := range batches {
		for j, span := range batch.Spans {
			if span, ok := spanMap[span.SpanID.ToOTELSpanID()]; ok {
				warnings := GetWarnings(span)
				batches[i].Spans[j].Warnings = append(batches[i].Spans[j].Warnings, warnings...)
			}
		}
	}
}
