// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adjuster

import (
	"fmt"

	"github.com/jaegertracing/jaeger/model"
)

// SpanReferences creates an adjuster that removes invalid span references, e.g. with traceID==0
func SpanReferences() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		adjuster := spanReferenceAdjuster{}
		for _, span := range trace.Spans {
			adjuster.adjust(span)
		}
		return trace, nil
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

func (s spanReferenceAdjuster) valid(ref *model.SpanRef) bool {
	return ref.TraceID.High != 0 || ref.TraceID.Low != 0
}
