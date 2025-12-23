// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

// GetFirstTrace extracts the first trace from OTLP traces
func GetFirstTrace(traces ptrace.Traces) *model.Trace {
	if traces.SpanCount() == 0 {
		return &model.Trace{}
	}
	batches := v1adapter.V1BatchesFromTraces(traces)
	if len(batches) == 0 {
		return &model.Trace{}
	}

	trace := &model.Trace{}
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if batch.Process != nil {
				processCopy := model.Process{
					ServiceName: batch.Process.ServiceName,
					Tags:        make([]model.KeyValue, len(batch.Process.Tags)),
				}
				copy(processCopy.Tags, batch.Process.Tags)
				span.Process = &processCopy
			}

			// Normalize nil slices to empty slices
			if span.Tags == nil {
				span.Tags = []model.KeyValue{}
			}
			if span.Logs == nil {
				span.Logs = []model.Log{}
			}
			if span.References == nil {
				span.References = []model.SpanRef{}
			}
		}
		trace.Spans = append(trace.Spans, batch.Spans...)
	}
	return trace
}

// OTLPTracesToV1Slice converts OTLP traces to v1 slice for comparison
func OTLPTracesToV1Slice(traces []ptrace.Traces) []*model.Trace {
	var result []*model.Trace
	for _, otlpTrace := range traces {
		trace := GetFirstTrace(otlpTrace)
		if len(trace.Spans) > 0 {
			result = append(result, trace)
		}
	}
	return result
}
