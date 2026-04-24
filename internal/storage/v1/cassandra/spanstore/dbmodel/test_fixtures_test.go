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
