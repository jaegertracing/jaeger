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
	someDBTags = []KeyValue{
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
	someDBTraceID = TraceIDFromDomain(someTraceID)
	someDBRefs    = []SpanRef{
		{
			RefType: "child-of",
			SpanID:  int64(someParentSpanID),
			TraceID: someDBTraceID,
		},
	}
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
