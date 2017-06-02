package json

import (
	"testing"
	"sort"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/model"
)

func CompareModelSpans(t *testing.T, expected *model.Span, actual *model.Span) {
	assert.Equal(t, expected.TraceID.Low, actual.TraceID.Low)
	assert.Equal(t, expected.TraceID.High, actual.TraceID.High)
	assert.Equal(t, expected.SpanID, actual.SpanID)
	assert.Equal(t, expected.OperationName, actual.OperationName)
	assert.Equal(t, expected.References, actual.References)
	assert.Equal(t, expected.Flags, actual.Flags)
	assert.Equal(t, expected.StartTime, actual.StartTime)
	assert.Equal(t, expected.Duration, actual.Duration)
	compareModelTags(t, expected.Tags, actual.Tags)
	compareModelLogs(t, expected.Logs, actual.Logs)
	compareModelProcess(t, expected.Process, actual.Process)
}

type TagByKey []model.KeyValue

func (t TagByKey) Len() int           { return len(t) }
func (t TagByKey) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t TagByKey) Less(i, j int) bool { return t[i].Key < t[j].Key }


func compareModelTags(t *testing.T, expected []model.KeyValue, actual []model.KeyValue) {
	sort.Sort(TagByKey(expected))
	sort.Sort(TagByKey(actual))
	assert.Equal(t, expected, actual)
	assert.Equal(t, len(expected), len(actual))
	for i := range expected {
		assert.Equal(t, expected[i].Key, actual[i].Key)
		assert.Equal(t, expected[i].VType, actual[i].VType)
		assert.Equal(t, expected[i].VStr, actual[i].VStr)
		assert.Equal(t, expected[i].VBlob, actual[i].VBlob)
		assert.Equal(t, expected[i].VNum, actual[i].VNum)
	}
}

type LogByTimestamp []model.Log

func (t LogByTimestamp) Len() int           { return len(t) }
func (t LogByTimestamp) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t LogByTimestamp) Less(i, j int) bool { return t[i].Timestamp.Before(t[j].Timestamp) }

// this function exists solely to make it easier for developer to find out where the difference is
func compareModelLogs(t *testing.T, expected []model.Log, actual []model.Log) {
	sort.Sort(LogByTimestamp(expected))
	sort.Sort(LogByTimestamp(actual))
	assert.Equal(t, len(expected), len(actual))
	for i := range expected {
		assert.Equal(t, expected[i].Timestamp, actual[i].Timestamp)
		compareModelTags(t, expected[i].Fields, actual[i].Fields)
	}
}

func compareModelProcess(t *testing.T, expected *model.Process, actual *model.Process) {
	assert.Equal(t, expected.ServiceName, actual.ServiceName)
	compareModelTags(t, expected.Tags, actual.Tags)
}
