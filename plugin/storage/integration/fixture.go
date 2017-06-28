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
