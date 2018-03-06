// Copyright (c) 2018 The Jaeger Authors.
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
	"sort"

	"github.com/jaegertracing/jaeger/model"
)

// MergeSpans returns an Adjuster that merges Jaeger spans with the same spanID.
// It skips merging Zipkin spans. Zipkin spans are defined as spans sharing the spanID and containing
// both client and server span.kind annotations.
// As Zipkin spans are always reported in entirety, we can assume that that the span.kind annotations are present.
// A drawback of this approach is that incomplete traces with duplicate Zipkin spans will be merged unnecessarily
//
// MergeSpans assumes that the duration field in a span is monotonically increasing for a spans with the
// same spanID. The span with the longest spanID wins, and is selected in entirety.
// TODO: Granular merging of spans
func MergeSpans() Adjuster {
	return Func(func(input *model.Trace) (*model.Trace, error) {

		IDToSpans := groupByIDs(input.Spans)

		if len(IDToSpans) == len(input.Spans) {
			return input, nil
		}

		trace := &model.Trace{}
		trace.Warnings = input.Warnings
		for _, spans := range IDToSpans {
			if isZipkin(spans) {
				trace.Spans = append(trace.Spans, spans...)
			} else {
				trace.Spans = append(trace.Spans, mergeSpans(spans))
			}
		}
		return trace, nil
	})
}

func groupByIDs(spans []*model.Span) map[model.SpanID][]*model.Span {
	IDToSpans := make(map[model.SpanID][]*model.Span)
	for _, span := range spans {
		if spans, ok := IDToSpans[span.SpanID]; ok {
			IDToSpans[span.SpanID] = append(spans, span)
		} else {
			IDToSpans[span.SpanID] = []*model.Span{span}
		}
	}
	return IDToSpans
}

func mergeSpans(spans []*model.Span) *model.Span {
	// This assumes that the duration field in a span is monotonically increasing
	// and uses it to break ties between spans.
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].Duration > spans[j].Duration
	})

	return spans[0]
}

func isZipkin(spans []*model.Span) bool {
	hasServer := false
	hasClient := false
	for _, span := range spans {
		if span.IsRPCClient() {
			hasClient = true
		}
		if span.IsRPCServer() {
			hasServer = true
		}
	}
	return hasServer && hasClient
}
