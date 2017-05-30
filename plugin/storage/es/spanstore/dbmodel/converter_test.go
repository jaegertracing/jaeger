package dbmodel

import (
	"testing"
	"encoding/hex"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

var (
	someTraceID       = model.TraceID{High: 22222, Low: 44444}
	someSpanID        = model.SpanID(3333)
	someParentSpanID  = model.SpanID(11111)
	someOperationName = "someOperationName"
	someStartTime     = model.EpochMicrosecondsAsTime(55555)
	someDuration      = model.MicrosecondsAsDuration(50000)
	someFlags         = model.Flags(1)
	someLogTimestamp  = model.EpochMicrosecondsAsTime(12345)
	someServiceName   = "someServiceName"

	someStringTagValue = "someTagValue"
	someBoolTagValue   = "true"
	someLongTagValue   = "123"
	someDoubleTagValue = "1.4"
	someBinaryTagValue = hex.EncodeToString([]byte("someBinaryValue"))
	someStringTagKey   = "someStringTag"
	someBoolTagKey     = "someBoolTag"
	someLongTagKey     = "someLongTag"
	someDoubleTagKey   = "someDoubleTag"
	someBinaryTagKey   = "someBinaryTag"
	someTags           = model.KeyValues{
		model.String(someStringTagKey, someStringTagValue),
		model.Bool(someBoolTagKey, true),
		model.Int64(someLongTagKey, int64(123)),
		model.Float64(someDoubleTagKey, float64(1.4)),
		model.Binary(someBinaryTagKey, []byte("someBinaryValue")),
	}
	someDBTags = []Tag{
		{
			Key:         someStringTagKey,
			TagType:   model.StringType.String(),
			Value: someStringTagValue,
		},
		{
			Key:       someBoolTagKey,
			TagType: model.BoolType.String(),
			Value: someBoolTagValue,
		},
		{
			Key:        someLongTagKey,
			TagType:  model.Int64Type.String(),
			Value: someLongTagValue,
		},
		{
			Key:          someDoubleTagKey,
			TagType:    model.Float64Type.String(),
			Value: someDoubleTagValue,
		},
		{
			Key:         someBinaryTagKey,
			TagType:   model.BinaryType.String(),
			Value: someBinaryTagValue,
		},
	}
	someLogs = []model.Log{
		{
			Timestamp: someLogTimestamp,
			Fields:    someTags,
		},
	}
	someDBLogs = []Log{
		{
			Timestamp: uint64(model.TimeAsEpochMicroseconds(someLogTimestamp)),
			Tags:    someDBTags,
		},
	}
	someRefs = []model.SpanRef{
		{
			TraceID: someTraceID,
			SpanID:  someParentSpanID,
			RefType: model.ChildOf,
		},
	}
	someDBProcess = Process{
		ServiceName: someServiceName,
		Tags:        someDBTags,
	}
	badDBTags = []Tag{
		{
			Key:       "sneh",
			TagType: "krustytheklown",
		},
	}
	someDBTraceID = TraceIDFromDomain(someTraceID)
	someDBRefs    = []Reference{
		{
			RefType: model.ChildOf.String(),
			SpanID:  SpanID(int64(someParentSpanID)),
			TraceID: someDBTraceID,
		},
	}
	notValidTagTypeErrStr = "not a valid ValueType string krustytheklown"
)

func getTestJaegerSpan() *model.Span {
	return &model.Span{
		TraceID:       someTraceID,
		SpanID:        someSpanID,
		ParentSpanID:  someParentSpanID,
		OperationName: someOperationName,
		References:    someRefs,
		Flags:         someFlags,
		StartTime:     someStartTime,
		Duration:      someDuration,
		Tags:          someTags,
		Logs:          someLogs,
		Process:       getTestJaegerProcess(),
	}
}

func getTestJaegerProcess() *model.Process {
	return &model.Process{
		ServiceName: someServiceName,
		Tags:        someTags,
	}
}

func getTestSpan() *Span {
	span := &Span{
		TraceID:       someDBTraceID,
		SpanID:        SpanID(int64(someSpanID)),
		ParentSpanID:  SpanID(int64(someParentSpanID)),
		OperationName: someOperationName,
		Flags:         uint32(someFlags),
		Timestamp:     uint64(model.TimeAsEpochMicroseconds(someStartTime)),
		Duration:      uint64(model.DurationAsMicroseconds(someDuration)),
		Tags:          someDBTags,
		Logs:          someDBLogs,
		References:    someDBRefs,
		Process:       someDBProcess,
	}
	return span
}

func getCustomSpan(dbTags []Tag, dbProcess Process, dbLogs []Log, dbRefs []Reference) *Span {
	span := getTestSpan()
	span.Tags = dbTags
	span.Logs = dbLogs
	span.References = dbRefs
	span.Process = dbProcess
	return span
}

func TestToSpan(t *testing.T) {
	expectedSpan := getTestSpan()
	actualDBSpan := FromDomain(getTestJaegerSpan())
	if !assert.EqualValues(t, expectedSpan, actualDBSpan) {
		for _, diff := range pretty.Diff(expectedSpan, actualDBSpan) {
			t.Log(diff)
		}
	}
}

func TestFromSpan(t *testing.T) {
	expectedSpan := getTestJaegerSpan()
	actualJSpan, err := ToDomain(getTestSpan())
	assert.NoError(t, err)
	if !assert.EqualValues(t, expectedSpan, actualJSpan) {
		for _, diff := range pretty.Diff(expectedSpan, actualJSpan) {
			t.Log(diff)
		}
	}
}

func TestFailingFromDBSpanBadTags(t *testing.T) {
	faultyDBTags := getCustomSpan(badDBTags, someDBProcess, someDBLogs, someDBRefs)
	failingDBSpanTransform(t, faultyDBTags, notValidTagTypeErrStr)
}

func TestFailingFromDBSpanBadLogs(t *testing.T) {
	faultyDBLogs := getCustomSpan(someDBTags, someDBProcess, []Log{
		{
			Timestamp: 0,
			Tags:    badDBTags,
		},
	}, someDBRefs)
	failingDBSpanTransform(t, faultyDBLogs, notValidTagTypeErrStr)
}

func TestFailingFromDBSpanBadProcess(t *testing.T) {
	faultyDBProcess := getCustomSpan(someDBTags, Process{
		ServiceName: someServiceName,
		Tags:        badDBTags,
	}, someDBLogs, someDBRefs)
	failingDBSpanTransform(t, faultyDBProcess, notValidTagTypeErrStr)
}

func TestFailingFromDBSpanBadRefs(t *testing.T) {
	faultyDBRefs := getCustomSpan(someDBTags, someDBProcess, someDBLogs, []Reference{
		{
			RefType: "makeOurOwnCasino",
			TraceID: someDBTraceID,
		},
	})
	failingDBSpanTransform(t, faultyDBRefs, "not a valid SpanRefType string makeOurOwnCasino")
}

func failingDBSpanTransform(t *testing.T, dbSpan *Span, errMsg string) {
	jSpan, err := ToDomain(dbSpan)
	assert.Nil(t, jSpan)
	assert.EqualError(t, err, errMsg)
}

func TestFailingFromDBLogs(t *testing.T) {
	someDBLogs := []Log{
		{
			Timestamp: 0,
			Tags: []Tag{
				{
					Key:       "sneh",
					TagType: "krustytheklown",
				},
			},
		},
	}
	jLogs, err := converter{}.fromDBLogs(someDBLogs)
	assert.Nil(t, jLogs)
	assert.EqualError(t, err, "not a valid ValueType string krustytheklown")
}

func TestDBTagTypeError(t *testing.T) {
	_, err := converter{}.fromDBTagOfType(&Tag{TagType: "x"}, model.ValueType(-1))
	assert.Equal(t, ErrUnknownKeyValueTypeFromES, err)
}

func TestGenerateHashCode(t *testing.T) {
	span1 := getTestJaegerSpan()
	span2 := getTestJaegerSpan()
	hc1, err1 := model.HashCode(span1)
	hc2, err2 := model.HashCode(span2)
	assert.Equal(t, hc1, hc2)
	assert.NoError(t, err1)
	assert.NoError(t, err2)

	span2.Tags = append(span2.Tags, model.String("xyz", "some new tag"))
	hc2, err2 = model.HashCode(span2)
	assert.NotEqual(t, hc1, hc2)
	assert.NoError(t, err2)
}
