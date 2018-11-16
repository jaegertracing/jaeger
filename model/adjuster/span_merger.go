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
	"github.com/jaegertracing/jaeger/model"
)

// MergeSpans returns an Adjuster that merges Jaeger spans with the same spanID.
// Duplicate spans that have conflicting span.kind annotations are not merged.
func MergeSpans() Adjuster {
	return Func(func(input *model.Trace) (*model.Trace, error) {

		IDToSpans := groupByIDs(input.Spans)

		if len(IDToSpans) == len(input.Spans) {
			return input, nil
		}

		trace := &model.Trace{}
		trace.Warnings = input.Warnings
		for _, spans := range IDToSpans {
			if isMergeable(spans) {
				trace.Spans = append(trace.Spans, mergeSpans(spans))
			} else {
				trace.Spans = append(trace.Spans, spans...)
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
	lastSpan := spans[0]
	for i := range spans {
		if !spans[i].GetIncomplete() {
			lastSpan = spans[i]
		}
	}
	warnings := []string{}
	tags := []model.KeyValue{}
	logs := []model.Log{}
	references := []model.SpanRef{}
	for _, span := range spans {
		//merge refs, tags, logs and warnings of all spans
		//take simple values from lastSpan
		if span.GetIncomplete() {
			references = append(references, span.GetReferences()...)
			tags = append(tags, span.GetTags()...)
			logs = append(logs, span.GetLogs()...)
			warnings = append(warnings, span.GetWarnings()...)
		}
	}
	//the default values for all types are null
	//this is why we check for array length otherwise we would
	//change semantics and an empty array would be the default value
	if len(references) > 0 {
		lastSpan.References = append(references, lastSpan.GetReferences()...)
	}
	if len(tags) > 0 {
		lastSpan.Tags = append(tags, lastSpan.GetTags()...)
	}
	if len(logs) > 0 {
		lastSpan.Logs = append(logs, lastSpan.GetLogs()...)
	}
	if len(warnings) > 0 {
		lastSpan.Warnings = append(warnings, lastSpan.GetWarnings()...)
	}

	return lastSpan
}

func isMergeable(spans []*model.Span) bool {
	// Checks that span.kind annotations are consistent, i.e all spans contain server/client or no span kind annotations
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
	return !(hasServer && hasClient)
}
