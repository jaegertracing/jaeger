package grpc

import (
	"testing"

	"github.com/jaegertracing/jaeger/model"
	"github.com/stretchr/testify/assert"
)

func TestTagsMerger_Merge_ClientOverride(t *testing.T) {
	agentTags := map[string]string{"agentKey1": "agentValue1", "commonKey": "commonValue1"}
	clientTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2"}

	expectedTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2", "agentKey1": "agentValue1"}
	tagsMerger := DeDupeClientTagsMerger()
	actualSpans := tagsMerger.Merge(makeModelKeyValue(clientTags), makeModelKeyValue(agentTags))

	assert.ElementsMatch(t, actualSpans, makeModelKeyValue(expectedTags))
}

func TestTagsMerger_Merge_JaegerBatchWithAgentOverride(t *testing.T) {
	agentTags := map[string]string{"agentKey1": "agentValue1", "commonKey": "commonValue1"}
	clientTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2"}

	expectedTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue1", "agentKey1": "agentValue1"}
	tagsMerger := DeDupeAgentTagsMerger()
	actualSpans := tagsMerger.Merge(makeModelKeyValue(clientTags), makeModelKeyValue(agentTags))

	assert.ElementsMatch(t, actualSpans, makeModelKeyValue(expectedTags))
}

func TestTagsMerger_Merge_JaegerBatchWithDuplicates(t *testing.T) {
	agentTags := map[string]string{"agentKey1": "agentValue1", "commonKey": "commonValue1"}
	clientTags := map[string]string{"clientKey1": "clientValue1", "commonKey": "commonValue2"}

	expectedTags := map[string]string{"clientKey1": "clientValue1", "agentKey1": "agentValue1", "commonKey": "commonValue1"}
	expectedModelKeyValue := append(makeModelKeyValue(expectedTags), []model.KeyValue{model.String("commonKey", "commonValue2")}...)

	tagsMerger := SimpleTagsMerger()
	actualSpans := tagsMerger.Merge(makeModelKeyValue(clientTags), makeModelKeyValue(agentTags))

	assert.ElementsMatch(t, actualSpans, expectedModelKeyValue)
}
