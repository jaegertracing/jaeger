// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

var _ TagFilter = &TagFilterDropAll{} // Check API compliance

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

func TestDropAll(t *testing.T) {
	tt := []struct {
		filter              *TagFilterDropAll
		expectedTags        []KeyValue
		expectedProcessTags []KeyValue
		expectedLogs        []KeyValue
	}{
		{
			filter:              NewTagFilterDropAll(false, false, false),
			expectedTags:        someDBTags,
			expectedProcessTags: someDBTags,
			expectedLogs:        someDBTags,
		},
		{
			filter:              NewTagFilterDropAll(true, false, false),
			expectedTags:        []KeyValue{},
			expectedProcessTags: someDBTags,
			expectedLogs:        someDBTags,
		},
		{
			filter:              NewTagFilterDropAll(false, true, false),
			expectedTags:        someDBTags,
			expectedProcessTags: []KeyValue{},
			expectedLogs:        someDBTags,
		},
		{
			filter:              NewTagFilterDropAll(false, false, true),
			expectedTags:        someDBTags,
			expectedProcessTags: someDBTags,
			expectedLogs:        []KeyValue{},
		},
		{
			filter:              NewTagFilterDropAll(true, false, true),
			expectedTags:        []KeyValue{},
			expectedProcessTags: someDBTags,
			expectedLogs:        []KeyValue{},
		},
		{
			filter:              NewTagFilterDropAll(true, true, true),
			expectedTags:        []KeyValue{},
			expectedProcessTags: []KeyValue{},
			expectedLogs:        []KeyValue{},
		},
	}

	for _, test := range tt {
		actualTags := test.filter.FilterTags(nil, someDBTags)
		assert.Equal(t, test.expectedTags, actualTags)

		actualProcessTags := test.filter.FilterProcessTags(nil, someDBTags)
		assert.Equal(t, test.expectedProcessTags, actualProcessTags)

		actualLogs := test.filter.FilterLogFields(nil, someDBTags)
		assert.Equal(t, test.expectedLogs, actualLogs)
	}
}
