// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"fmt"
	"iter"

	jaegerTranslator "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
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

// V1TracesFromSeq2 converts an iterator of ptrace.Traces chunks into v1 traces.
// If maxTraceSize > 0, traces exceeding that number of spans will be truncated with warnings.
func V1TracesFromSeq2(otelSeq iter.Seq2[[]ptrace.Traces, error], maxTraceSize ...int) ([]*model.Trace, error) {
	limit := 0
	if len(maxTraceSize) > 0 {
		limit = maxTraceSize[0]
	}

	// Early return for no limit case to avoid unnecessary processing
	if limit <= 0 {
		var (
			jaegerTraces []*model.Trace
			iterErr      error
		)
		jptrace.AggregateTraces(otelSeq)(func(otelTrace ptrace.Traces, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			trace := modelTraceFromOtelTrace(otelTrace)
			jaegerTraces = append(jaegerTraces, trace)
			return true
		})
		if iterErr != nil {
			return nil, iterErr
		}
		return jaegerTraces, nil
	}

	var (
		jaegerTraces []*model.Trace
		iterErr      error
	)

	// Apply trace size limit incrementally during sequence consumption
	limitedSeq := applyTraceSizeLimit(otelSeq, limit)
	jptrace.AggregateTraces(limitedSeq)(func(otelTrace ptrace.Traces, err error) bool {
		if err != nil {
			iterErr = err
			return false
		}
		trace := modelTraceFromOtelTrace(otelTrace)
		jaegerTraces = append(jaegerTraces, trace)
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	return jaegerTraces, nil
}

// applyTraceSizeLimit applies trace size limits incrementally during sequence consumption.
// This prevents loading large traces entirely into memory before applying the limit.
func applyTraceSizeLimit(otelSeq iter.Seq2[[]ptrace.Traces, error], maxTraceSize int) iter.Seq2[[]ptrace.Traces, error] {
	if maxTraceSize <= 0 {
		return otelSeq
	}

	return func(yield func(traces []ptrace.Traces, err error) bool) {
		otelSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				return yield(traces, err)
			}

			// Pre-allocate slice with known capacity for better performance
			limitedTraces := make([]ptrace.Traces, 0, len(traces))

			// Process each trace in the batch and apply size limits incrementally
			for _, trace := range traces {
				limitedTrace := applyTraceSizeLimitToSingleTrace(trace, maxTraceSize)
				limitedTraces = append(limitedTraces, limitedTrace)
			}

			return yield(limitedTraces, nil)
		})
	}
}

// applyTraceSizeLimitToSingleTrace applies size limit to a single trace incrementally.
// Optimized to avoid counting all spans upfront and use early returns.
func applyTraceSizeLimitToSingleTrace(trace ptrace.Traces, maxTraceSize int) ptrace.Traces {
	if maxTraceSize <= 0 {
		return trace
	}

	// Quick check: if the trace has no resource spans, return early
	resources := trace.ResourceSpans()
	if resources.Len() == 0 {
		return trace
	}

	// Pre-allocate with estimated capacity for better performance
	estimatedSpans := 0
	for i := 0; i < resources.Len(); i++ {
		scopes := resources.At(i).ScopeSpans()
		for j := 0; j < scopes.Len(); j++ {
			estimatedSpans += scopes.At(j).Spans().Len()
		}
	}

	// If trace is within limit, return as-is to avoid unnecessary copying
	if estimatedSpans <= maxTraceSize {
		return trace
	}

	// Create a new trace with limited spans, counting as we go
	limitedTrace := ptrace.NewTraces()

	spansAdded := 0

	for i := 0; i < resources.Len() && spansAdded < maxTraceSize; i++ {
		resource := resources.At(i)
		limitedResource := limitedTrace.ResourceSpans().AppendEmpty()
		resource.Resource().CopyTo(limitedResource.Resource())

		scopes := resource.ScopeSpans()
		for j := 0; j < scopes.Len() && spansAdded < maxTraceSize; j++ {
			scope := scopes.At(j)
			limitedScope := limitedResource.ScopeSpans().AppendEmpty()
			scope.Scope().CopyTo(limitedScope.Scope())

			spans := scope.Spans()
			scopeSpanCount := spans.Len()

			for k := 0; k < scopeSpanCount && spansAdded < maxTraceSize; k++ {
				span := spans.At(k)
				limitedSpan := limitedScope.Spans().AppendEmpty()
				span.CopyTo(limitedSpan)
				spansAdded++

				// Add warning to the first span if this is a truncated trace
				if spansAdded == 1 {
					warningTag := pcommon.NewValueStr(fmt.Sprintf("Trace truncated: only first %d spans loaded (total spans: %d)", maxTraceSize, estimatedSpans))
					limitedSpan.Attributes().PutStr("jaeger.warning", warningTag.Str())
				}
			}
		}
	}

	return limitedTrace
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
				span.Process.Tags = make([]model.KeyValue, 0)
			}

			if span.References == nil {
				span.References = make([]model.SpanRef, 0)
			}
			if span.Tags == nil {
				span.Tags = make([]model.KeyValue, 0)
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
