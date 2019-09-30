package grpc

import (
	"testing"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/model"
	"github.com/stretchr/testify/assert"
)

func TestTagsMerger_Merge_EmptyTags(t *testing.T) {
	tags := map[string]string{}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), OperationName: "jonatan"}}
	tagsMerger := tagsMerger{agentTags: makeModelKeyValue(tags), duplicateTagsPolicy: reporter.Duplicate}
	actualSpans, _ := tagsMerger.Merge(spans, nil)
	assert.Equal(t, spans, actualSpans)
}

func TestTagsMerger_Merge_ZipkinBatch(t *testing.T) {
	tags := map[string]string{"key": "value"}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), OperationName: "jonatan", Process: &model.Process{ServiceName: "spring"}}}

	expectedSpans := []*model.Span{
		{
			TraceID:       model.NewTraceID(0, 1),
			SpanID:        model.NewSpanID(2),
			OperationName: "jonatan",
			Process:       &model.Process{ServiceName: "spring", Tags: []model.KeyValue{model.String("key", "value")}},
		},
	}
	tagsMerger := tagsMerger{agentTags: makeModelKeyValue(tags), duplicateTagsPolicy: reporter.Duplicate}
	actualSpans, _ := tagsMerger.Merge(spans, nil)

	assert.Equal(t, expectedSpans, actualSpans)
}

func TestTagsMerger_Merge_JaegerBatch(t *testing.T) {
	tags := map[string]string{"key": "value"}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), OperationName: "jonatan"}}
	process := &model.Process{ServiceName: "spring"}

	expectedProcess := &model.Process{ServiceName: "spring", Tags: []model.KeyValue{model.String("key", "value")}}
	tagsMerger := tagsMerger{agentTags: makeModelKeyValue(tags), duplicateTagsPolicy: reporter.Duplicate}
	_, actualProcess := tagsMerger.Merge(spans, process)

	assert.Equal(t, expectedProcess, actualProcess)
}

func TestTagsMerger_Merge_JaegerBatchWithClientOverride(t *testing.T) {
	agentTags := map[string]string{"agentKey1": "agentValue1", "commonKey": "commonValue1"}
	clientTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2"}

	process := &model.Process{Tags: makeModelKeyValue(clientTags)}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), Process: process, OperationName: "jonatan"}}

	expectedTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2", "agentKey1": "agentValue1"}

	tagsMerger := tagsMerger{agentTags: makeModelKeyValue(agentTags), duplicateTagsPolicy: reporter.Client}
	actualSpans, _ := tagsMerger.Merge(spans, nil)

	assert.ElementsMatch(t, actualSpans[0].Process.Tags, makeModelKeyValue(expectedTags))
}

func TestTagsMerger_Merge_JaegerBatchWithAgentOverride(t *testing.T) {
	agentTags := map[string]string{"agentKey1": "agentValue1", "commonKey": "commonValue1"}
	clientTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2"}

	process := &model.Process{Tags: makeModelKeyValue(clientTags)}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), Process: process, OperationName: "jonatan"}}

	expectedTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue1", "agentKey1": "agentValue1"}

	tagsMerger := tagsMerger{agentTags: makeModelKeyValue(agentTags), duplicateTagsPolicy: reporter.Agent}
	actualSpans, _ := tagsMerger.Merge(spans, nil)

	assert.ElementsMatch(t, actualSpans[0].Process.Tags, makeModelKeyValue(expectedTags))
}

func TestTagsMerger_Merge_JaegerBatchWithDuplicates(t *testing.T) {
	agentTags := map[string]string{"agentKey1": "agentValue1", "commonKey": "commonValue1"}
	clientTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2"}

	process := &model.Process{Tags: makeModelKeyValue(clientTags)}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), Process: process, OperationName: "jonatan"}}

	expectedTags := map[string]string{"clientKey1": "clientValue1", "agentKey1": "agentValue1", "commonKey": "commonValue1"}
	expectedModelKeyValue := append(makeModelKeyValue(expectedTags), []model.KeyValue{model.String("commonKey", "commonValue2")}...)

	tagsMerger := tagsMerger{agentTags: makeModelKeyValue(agentTags), duplicateTagsPolicy: reporter.Duplicate}
	actualSpans, _ := tagsMerger.Merge(spans, nil)

	assert.ElementsMatch(t, actualSpans[0].Process.Tags, expectedModelKeyValue)
}
