// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

var (
	someTraceID       = model.NewTraceID(22222, 44444)
	someSpanID        = model.SpanID(3333)
	someParentSpanID  = model.SpanID(11111)
	someOperationName = "someOperationName"
	someStartTime     = model.EpochMicrosecondsAsTime(55555)
	someDuration      = model.MicrosecondsAsDuration(50000)
	someFlags         = model.Flags(1)
	someLogTimestamp  = model.EpochMicrosecondsAsTime(12345)
	someServiceName   = "someServiceName"

	someStringTagValue = "someTagValue"
	someBoolTagValue   = true
	someLongTagValue   = int64(123)
	someDoubleTagValue = float64(1.4)
	someBinaryTagValue = []byte("someBinaryValue")
	someStringTagKey   = "someStringTag"
	someBoolTagKey     = "someBoolTag"
	someLongTagKey     = "someLongTag"
	someDoubleTagKey   = "someDoubleTag"
	someBinaryTagKey   = "someBinaryTag"
	someTags           = model.KeyValues{
		model.String(someStringTagKey, someStringTagValue),
		model.Bool(someBoolTagKey, someBoolTagValue),
		model.Int64(someLongTagKey, someLongTagValue),
		model.Float64(someDoubleTagKey, someDoubleTagValue),
		model.Binary(someBinaryTagKey, someBinaryTagValue),
	}
	someWarnings = []string{"warning text 1", "warning text 2"}
	someDBTags   = []KeyValue{
		{
			Key:         someStringTagKey,
			ValueType:   StringType,
			ValueString: someStringTagValue,
		},
		{
			Key:       someBoolTagKey,
			ValueType: BoolType,
			ValueBool: someBoolTagValue,
		},
		{
			Key:        someLongTagKey,
			ValueType:  Int64Type,
			ValueInt64: someLongTagValue,
		},
		{
			Key:          someDoubleTagKey,
			ValueType:    Float64Type,
			ValueFloat64: someDoubleTagValue,
		},
		{
			Key:         someBinaryTagKey,
			ValueType:   BinaryType,
			ValueBinary: someBinaryTagValue,
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
			Timestamp: int64(model.TimeAsEpochMicroseconds(someLogTimestamp)),
			Fields:    someDBTags,
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
	badDBTags = []KeyValue{
		{
			Key:       "sneh",
			ValueType: "krustytheklown",
		},
	}
	someDBTraceID = TraceIDFromDomain(someTraceID)
	someDBRefs    = []SpanRef{
		{
			RefType: "child-of",
			SpanID:  int64(someParentSpanID),
			TraceID: someDBTraceID,
		},
	}
	notValidTagTypeErrStr = "invalid ValueType in"
)

func getTestJaegerSpan() *model.Span {
	return &model.Span{
		TraceID:       someTraceID,
		SpanID:        someSpanID,
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
		SpanID:        int64(someSpanID),
		OperationName: someOperationName,
		Flags:         int32(someFlags),
		StartTime:     int64(model.TimeAsEpochMicroseconds(someStartTime)),
		Duration:      int64(model.DurationAsMicroseconds(someDuration)),
		Tags:          someDBTags,
		Logs:          someDBLogs,
		Refs:          someDBRefs,
		Process:       someDBProcess,
		ServiceName:   someServiceName,
	}
	// there is no way to validate if the hash code is "correct" or not,
	// other than comparing it with some magic number that keeps changing
	// as the model changes. So let's just make sure the code is being
	// calculated during the conversion.
	spanHash, _ := model.HashCode(getTestJaegerSpan())
	span.SpanHash = int64(spanHash)
	return span
}

func getCustomSpan(dbTags []KeyValue, dbProcess Process, dbLogs []Log, dbRefs []SpanRef) *Span {
	span := getTestSpan()
	span.Tags = dbTags
	span.Logs = dbLogs
	span.Refs = dbRefs
	span.Process = dbProcess
	return span
}

func getTestUniqueTags() []TagInsertion {
	return []TagInsertion{
		{ServiceName: "someServiceName", TagKey: "someBoolTag", TagValue: "true"},
		{ServiceName: "someServiceName", TagKey: "someDoubleTag", TagValue: "1.4"},
		{ServiceName: "someServiceName", TagKey: "someLongTag", TagValue: "123"},
		{ServiceName: "someServiceName", TagKey: "someStringTag", TagValue: "someTagValue"},
	}
}

func TestToSpan(t *testing.T) {
	expectedSpan := getTestSpan()
	actualDBSpan := FromDomain(getTestJaegerSpan())
	if !assert.Equal(t, expectedSpan, actualDBSpan) {
		for _, diff := range pretty.Diff(expectedSpan, actualDBSpan) {
			t.Log(diff)
		}
	}
}

func TestFromSpan(t *testing.T) {
	for _, testParentID := range []bool{false, true} {
		testDBSpan := getTestSpan()
		if testParentID {
			testDBSpan.ParentID = testDBSpan.Refs[0].SpanID
			testDBSpan.Refs = nil
		}
		expectedSpan := getTestJaegerSpan()
		actualJSpan, err := ToDomain(testDBSpan)
		require.NoError(t, err)
		if !assert.Equal(t, expectedSpan, actualJSpan) {
			for _, diff := range pretty.Diff(expectedSpan, actualJSpan) {
				t.Log(diff)
			}
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
			Fields:    badDBTags,
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
	faultyDBRefs := getCustomSpan(someDBTags, someDBProcess, someDBLogs, []SpanRef{
		{
			RefType: "makeOurOwnCasino",
			TraceID: someDBTraceID,
		},
	})
	failingDBSpanTransform(t, faultyDBRefs, "invalid SpanRefType in")
}

func failingDBSpanTransform(t *testing.T, dbSpan *Span, errMsg string) {
	jSpan, err := ToDomain(dbSpan)
	assert.Nil(t, jSpan)
	assert.ErrorContains(t, err, errMsg)
}

func TestFailingFromDBLogs(t *testing.T) {
	someDBLogs := []Log{
		{
			Timestamp: 0,
			Fields: []KeyValue{
				{
					Key:       "sneh",
					ValueType: "krustytheklown",
				},
			},
		},
	}
	jLogs, err := converter{}.fromDBLogs(someDBLogs)
	assert.Nil(t, jLogs)
	assert.ErrorContains(t, err, notValidTagTypeErrStr)
}

func TestDBTagTypeError(t *testing.T) {
	_, err := converter{}.fromDBTag(&KeyValue{ValueType: "x"})
	assert.ErrorContains(t, err, notValidTagTypeErrStr)
}

func TestGenerateHashCode(t *testing.T) {
	span1 := getTestJaegerSpan()
	span2 := getTestJaegerSpan()
	hc1, err1 := model.HashCode(span1)
	hc2, err2 := model.HashCode(span2)
	assert.Equal(t, hc1, hc2)
	require.NoError(t, err1)
	require.NoError(t, err2)

	span2.Tags = append(span2.Tags, model.String("xyz", "some new tag"))
	hc2, err2 = model.HashCode(span2)
	assert.NotEqual(t, hc1, hc2)
	require.NoError(t, err2)
}

func TestFromDBTagsWithoutWarnings(t *testing.T) {
	span := getTestJaegerSpan()
	dbSpan := FromDomain(span)

	tags, err := converter{}.fromDBTags(dbSpan.Tags)
	require.NoError(t, err)
	assert.Equal(t, tags, span.Tags)
}

func TestFromDBTagsWithWarnings(t *testing.T) {
	span := getTestJaegerSpan()
	span.Warnings = someWarnings
	dbSpan := FromDomain(span)

	tags, err := converter{}.fromDBTags(dbSpan.Tags)
	require.NoError(t, err)
	assert.Equal(t, tags, span.Tags)
}

func TestFromDBLogsWithWarnings(t *testing.T) {
	span := getTestJaegerSpan()
	span.Warnings = someWarnings
	dbSpan := FromDomain(span)

	logs, err := converter{}.fromDBLogs(dbSpan.Logs)
	require.NoError(t, err)
	assert.Equal(t, logs, span.Logs)
}

func TestFromDBProcessWithWarnings(t *testing.T) {
	span := getTestJaegerSpan()
	span.Warnings = someWarnings
	dbSpan := FromDomain(span)

	process, err := converter{}.fromDBProcess(dbSpan.Process)
	require.NoError(t, err)
	assert.Equal(t, process, span.Process)
}

func TestFromDBWarnings(t *testing.T) {
	span := getTestJaegerSpan()
	span.Warnings = someWarnings
	dbSpan := FromDomain(span)

	warnings, err := converter{}.fromDBWarnings(dbSpan.Tags)
	require.NoError(t, err)
	assert.Equal(t, warnings, span.Warnings)
}

func TestFailingFromDBWarnings(t *testing.T) {
	badDBWarningTags := []KeyValue{{Key: warningStringPrefix + "1", ValueType: "invalidValueType"}}
	span := getCustomSpan(badDBWarningTags, someDBProcess, someDBLogs, someDBRefs)
	failingDBSpanTransform(t, span, notValidTagTypeErrStr)
}

func TestFromDBTag_DefaultCase(t *testing.T) {
	tag := &KeyValue{
		Key:         "test-key",
		ValueType:   "unknown-type",
		ValueString: "test-value",
	}

	converter := converter{}
	result, err := converter.fromDBTag(tag)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ValueType")
	assert.Equal(t, model.KeyValue{}, result)
}
