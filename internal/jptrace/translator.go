package jptrace

import (
	"github.com/jaegertracing/jaeger/model"
	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func ProtoFromTraces(traces ptrace.Traces) []*model.Batch {
	batches := otlp2jaeger.ProtoFromTraces(traces)
	for i, batch := range batches {
		scopes := traces.ResourceSpans().At(i).ScopeSpans()
		spanIndex := 0
		for j := 0; j < scopes.Len(); j++ {
			spans := scopes.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				// add warnings
				warnings := GetWarnings(span)
				if len(warnings) > 0 {
					batch.Spans[spanIndex].Warnings = warnings
				}
				spanIndex++
			}
		}
	}
	return batches
}
