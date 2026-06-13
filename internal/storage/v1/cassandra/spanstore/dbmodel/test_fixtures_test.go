// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "github.com/jaegertracing/jaeger-idl/model/v1"

var (
	someSpanID        = int64(3333)
	someParentSpanID  = int64(11111)
	someOperationName = "someOperationName"
	someStartTime     = model.EpochMicrosecondsAsTime(55555)
	someDuration      = model.MicrosecondsAsDuration(50000)
	someFlags         = int32(1)
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
	someDBTags         = []KeyValue{
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
	someDBLogs = []Log{
		{
			Timestamp: int64(model.TimeAsEpochMicroseconds(someLogTimestamp)),
			Fields:    someDBTags,
		},
	}
	someDBProcess = Process{
		ServiceName: someServiceName,
		Tags:        someDBTags,
	}
	someDBTraceID = TraceID{2, 2, 2, 2, 2, 4, 4, 4, 4}
	someDBRefs    = []SpanRef{
		{
			RefType: "child-of",
			SpanID:  someParentSpanID,
			TraceID: someDBTraceID,
		},
	}
)

func getTestSpan() *Span {
	span := &Span{
		TraceID:       someDBTraceID,
		SpanID:        someSpanID,
		OperationName: someOperationName,
		Flags:         someFlags,
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
	spanHash, _ := model.HashCode(span)
	span.SpanHash = int64(spanHash)
	return span
}
