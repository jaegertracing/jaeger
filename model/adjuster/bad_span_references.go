// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"fmt"

	"github.com/jaegertracing/jaeger/model"
)

// SpanReferences creates an adjuster that removes invalid span references, e.g. with traceID==0
func SpanReferences() Adjuster {
	return Func(func(trace *model.Trace) *model.Trace {
		adjuster := spanReferenceAdjuster{}
		for _, span := range trace.Spans {
			adjuster.adjust(span)
		}
		return trace
	})
}

type spanReferenceAdjuster struct{}

func (s spanReferenceAdjuster) adjust(span *model.Span) *model.Span {
	foundInvalid := false
	for i := range span.References {
		if !s.valid(&span.References[i]) {
			foundInvalid = true
			break
		}
	}
	if !foundInvalid {
		return span
	}
	references := make([]model.SpanRef, 0, len(span.References)-1)
	for i := range span.References {
		if !s.valid(&span.References[i]) {
			span.Warnings = append(span.Warnings, fmt.Sprintf("Invalid span reference removed %+v", span.References[i]))
			continue
		}
		references = append(references, span.References[i])
	}
	span.References = references
	return span
}

func (spanReferenceAdjuster) valid(ref *model.SpanRef) bool {
	return ref.TraceID.High != 0 || ref.TraceID.Low != 0
}
