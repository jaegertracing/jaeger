// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"iter"

	jaegertranslator "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// V1BatchesFromTraces converts OpenTelemetry traces (ptrace.Traces)
// to Jaeger model batches ([]*model.Batch).
func V1BatchesFromTraces(traces ptrace.Traces) []*model.Batch {
	batches := jaegertranslator.ProtoFromTraces(traces)
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
	traces, _ := jaegertranslator.ProtoToTraces(batches) // never returns an error
	spanMap := jptrace.SpanMap(traces, func(s ptrace.Span) pcommon.SpanID {
		return s.SpanID()
	})
	transferWarningsToOTLPSpans(batches, spanMap)
	return traces
}

// V1TracesFromSeq2 converts an interator of ptrace.Traces chunks into v1 traces.
// If maxTraceSize > 0, traces exceeding that number of spans will be truncated.
func V1TracesFromSeq2(otelSeq iter.Seq2[[]ptrace.Traces, error], maxTraceSize int) ([]*model.Trace, error) {
	// Apply size limiting if needed
	if maxTraceSize > 0 {
		otelSeq = applyTraceSizeLimit(otelSeq, maxTraceSize)
	}

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

// applyTraceSizeLimit applies a per-trace span budget while consuming the iterator.
// Once the budget is reached for a logical trace, remaining chunks of that trace are not yielded,
// and if the current chunk exceeds the remaining budget it is truncated to the exact number of remaining spans.
func applyTraceSizeLimit(otelSeq iter.Seq2[[]ptrace.Traces, error], maxTraceSize int) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		spansProcessed := 0
		currentTraceID := pcommon.NewTraceIDEmpty()
		truncated := false

		otelSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				return yield(nil, err)
			}

			limited := make([]ptrace.Traces, 0, len(traces))
			for _, tr := range traces {
				// Single-pass: extract trace ID and count spans in one iteration
				tid, spanCount := extractTraceIDAndCount(tr)
				if tid != (pcommon.TraceID{}) && tid != currentTraceID {
					currentTraceID = tid
					spansProcessed = 0
					truncated = false
				}

				if truncated {
					// already reached limit for this trace; skip this trace
					continue
				}

				if spansProcessed+spanCount > maxTraceSize {
					remaining := maxTraceSize - spansProcessed
					if remaining > 0 {
						limited = append(limited, truncateTraceToLimit(tr, remaining))
						spansProcessed += remaining
					}
					truncated = true
					// Continue to next trace in the same batch
					continue
				}

				limited = append(limited, tr)
				spansProcessed += spanCount
			}

			if len(limited) > 0 {
				if !yield(limited, nil) {
					return false
				}
			}
			return true // Always continue to process more batches
		})
	}
}

// extractTraceIDAndCount extracts the first trace ID and counts spans in a single pass.
func extractTraceIDAndCount(tr ptrace.Traces) (pcommon.TraceID, int) {
	tid := pcommon.NewTraceIDEmpty()
	count := 0
	rs := tr.ResourceSpans()
	for i := 0; i < rs.Len(); i++ {
		ss := rs.At(i).ScopeSpans()
		for j := 0; j < ss.Len(); j++ {
			spans := ss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				if tid == (pcommon.TraceID{}) {
					tid = spans.At(k).TraceID()
				}
				count++
			}
		}
	}
	return tid, count
}

// truncateTraceToLimit returns a new ptrace.Traces containing at most remaining spans, preserving
// resource and scope attributes for included spans only.
func truncateTraceToLimit(tr ptrace.Traces, remaining int) ptrace.Traces {
	out := ptrace.NewTraces()
	rs := tr.ResourceSpans()
	copied := 0
	for i := 0; i < rs.Len() && copied < remaining; i++ {
		inRes := rs.At(i)
		outRes := out.ResourceSpans().AppendEmpty()
		inRes.Resource().CopyTo(outRes.Resource())
		ss := inRes.ScopeSpans()
		for j := 0; j < ss.Len() && copied < remaining; j++ {
			inScope := ss.At(j)
			outScope := outRes.ScopeSpans().AppendEmpty()
			inScope.Scope().CopyTo(outScope.Scope())
			spans := inScope.Spans()
			for k := 0; k < spans.Len() && copied < remaining; k++ {
				span := spans.At(k)
				span.CopyTo(outScope.Spans().AppendEmpty())
				copied++
			}
		}
	}
	return out
}

func V1TraceIDsFromSeq2(traceIDsIter iter.Seq2[[]tracestore.FoundTraceID, error]) ([]model.TraceID, error) {
	var (
		iterErr       error
		modelTraceIDs []model.TraceID
	)
	traceIDsIter(func(traceIDs []tracestore.FoundTraceID, err error) bool {
		if err != nil {
			iterErr = err
			return false
		}
		for _, traceID := range traceIDs {
			modelTraceIDs = append(modelTraceIDs, ToV1TraceID(traceID.TraceID))
		}
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	return modelTraceIDs, nil
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

			if span.Process.Tags == nil {
				span.Process.Tags = []model.KeyValue{}
			}

			if span.References == nil {
				span.References = []model.SpanRef{}
			}
			if span.Tags == nil {
				span.Tags = []model.KeyValue{}
			}
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
				if span, ok := spanMap[ToV1SpanID(otelSpan.SpanID())]; ok {
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
			if otelSpan, ok := spanMap[FromV1SpanID(span.SpanID)]; ok {
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
