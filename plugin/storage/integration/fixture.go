// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package integration

import (
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
)

// CheckTraceWithQuery returns true if the trace fits the query, false if not.
func CheckTraceWithQuery(trace *model.Trace, traceQuery *spanstore.TraceQueryParameters) bool {
	for _, span := range trace.Spans {
		if checkSpanWithQuery(span, traceQuery) {
			return true
		}
	}
	return false
}

func checkSpanWithQuery(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return matchDurationQueryWithSpan(span, traceQuery) &&
		matchServiceNameQueryWithSpan(span, traceQuery) &&
		matchOperationNameQueryWithSpan(span, traceQuery) &&
		matchStartTimeQueryWithSpan(span, traceQuery) &&
		matchTagsQueryWithSpan(span, traceQuery)
}

func matchServiceNameQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return span.Process.ServiceName == traceQuery.ServiceName
}

func matchOperationNameQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return traceQuery.OperationName == "" || span.OperationName == traceQuery.OperationName
}

func matchStartTimeQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return traceQuery.StartTimeMin.Before(span.StartTime) && span.StartTime.Before(traceQuery.StartTimeMax)
}

func matchDurationQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	if traceQuery.DurationMin == 0 && traceQuery.DurationMax == 0 {
		return true
	}
	return traceQuery.DurationMin <= span.Duration && span.Duration <= traceQuery.DurationMax
}

func matchTagsQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	if len(traceQuery.Tags) == 0 {
		return true
	}
	return spanHasAllTags(span, traceQuery.Tags)
}

func spanHasAllTags(span *model.Span, tags map[string]string) bool {
	for key, val := range tags {
		if !checkAllSpots(span, key, val) {
			return false
		}
	}
	return true
}

func checkAllSpots(span *model.Span, key string, val string) bool {
	tag, found := span.Tags.FindByKey(key)
	if found && tag.AsString() == val {
		return true
	}
	if span.Process != nil {
		tag, found = span.Process.Tags.FindByKey(key)
		if found && tag.AsString() == val {
			return true
		}
	}
	for _, log := range span.Logs {
		if len(log.Fields) == 0 {
			continue
		}
		tag, found = model.KeyValues(log.Fields).FindByKey(key)
		if found && tag.AsString() == val {
			return true
		}
	}
	return false
}
