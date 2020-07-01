// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package esmodeltranslator

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.opentelemetry.io/collector/translator/conventions"

	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

var (
	traceID = []byte("0123456789abcdef")
	spanID  = []byte("01234567")
)

func TestAttributeToKeyValue(t *testing.T) {
	tests := []struct {
		key      string
		attr     pdata.AttributeValue
		keyValue dbmodel.KeyValue
	}{
		{
			key:      "foo",
			attr:     pdata.NewAttributeValueString("bar"),
			keyValue: dbmodel.KeyValue{Key: "foo", Value: "bar", Type: dbmodel.StringType},
		},
		{
			key:      "foo",
			attr:     pdata.NewAttributeValueBool(true),
			keyValue: dbmodel.KeyValue{Key: "foo", Value: "true", Type: dbmodel.BoolType},
		},
		{
			key:      "foo",
			attr:     pdata.NewAttributeValueBool(false),
			keyValue: dbmodel.KeyValue{Key: "foo", Value: "false", Type: dbmodel.BoolType},
		},
		{
			key:      "foo",
			attr:     pdata.NewAttributeValueInt(15),
			keyValue: dbmodel.KeyValue{Key: "foo", Value: "15", Type: dbmodel.Int64Type},
		},
		{
			key:      "foo",
			attr:     pdata.NewAttributeValueDouble(16.42),
			keyValue: dbmodel.KeyValue{Key: "foo", Value: "16.42", Type: dbmodel.Float64Type},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s:%v", test.keyValue.Key, test.keyValue.Value), func(t *testing.T) {
			keyValue := attributeToKeyValue(test.key, test.attr)
			assert.Equal(t, test.keyValue, keyValue)
		})
	}
}

func TestTagMapValue(t *testing.T) {
	tests := []struct {
		attr  pdata.AttributeValue
		value interface{}
	}{
		{
			attr:  pdata.NewAttributeValueString("foo"),
			value: "foo",
		},
		{
			attr:  pdata.NewAttributeValueBool(true),
			value: true,
		},
		{
			attr:  pdata.NewAttributeValueInt(15),
			value: int64(15),
		},
		{
			attr:  pdata.NewAttributeValueDouble(123.66),
			value: float64(123.66),
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v", test.value), func(t *testing.T) {
			val := attributeValueToInterface(test.attr)
			assert.Equal(t, test.value, val)
		})
	}
}

func TestConvertSpan(t *testing.T) {
	traces := traces("myservice")
	resource := traces.ResourceSpans().At(0).Resource()
	resource.Attributes().InsertDouble("num", 16.66)
	span := addSpan(traces, "root", traceID, spanID)
	span.SetKind(pdata.SpanKindCLIENT)
	span.Status().InitEmpty()
	span.Status().SetCode(1)
	span.Status().SetMessage("messagetext")
	span.SetStartTime(pdata.TimestampUnixNano(1000000))
	span.SetEndTime(pdata.TimestampUnixNano(2000000))
	span.Attributes().Insert("foo", pdata.NewAttributeValueBool(true))
	span.Attributes().Insert("toTagMap", pdata.NewAttributeValueString("val"))
	span.Events().Resize(1)
	span.Events().At(0).SetName("eventName")
	span.Events().At(0).SetTimestamp(500000)
	span.Events().At(0).Attributes().InsertString("foo", "bar")
	span.SetParentSpanID(spanID)
	span.Links().Resize(1)
	span.Links().At(0).InitEmpty()
	span.Links().At(0).SetSpanID(pdata.NewSpanID(spanID))
	span.Links().At(0).SetTraceID(pdata.NewTraceID(traceID))

	c := &Translator{
		tagKeysAsFields: map[string]bool{"toTagMap": true},
	}
	spans, err := c.ConvertSpans(traces)
	require.NoError(t, err)
	assert.Equal(t, 1, len(spans))
	assert.Equal(t, &dbmodel.Span{
		TraceID:         "30313233343536373839616263646566",
		SpanID:          "3031323334353637",
		StartTime:       1000,
		Duration:        1000,
		OperationName:   "root",
		StartTimeMillis: 1,
		Tags: []dbmodel.KeyValue{
			{Key: "span.kind", Type: dbmodel.StringType, Value: "client"},
			{Key: "status.code", Type: dbmodel.StringType, Value: "Cancelled"},
			{Key: "error", Type: dbmodel.BoolType, Value: "true"},
			{Key: "status.message", Type: dbmodel.StringType, Value: "messagetext"},
			{Key: "foo", Type: dbmodel.BoolType, Value: "true"}},
		Tag: map[string]interface{}{"toTagMap": "val"},
		Logs: []dbmodel.Log{{Fields: []dbmodel.KeyValue{
			{Key: "event", Value: "eventName", Type: dbmodel.StringType},
			{Key: "foo", Value: "bar", Type: dbmodel.StringType}}, Timestamp: 500}},
		References: []dbmodel.Reference{
			{SpanID: "3031323334353637", TraceID: "30313233343536373839616263646566", RefType: dbmodel.ChildOf},
			{SpanID: "3031323334353637", TraceID: "30313233343536373839616263646566", RefType: dbmodel.FollowsFrom}},
		Process: dbmodel.Process{
			ServiceName: "myservice",
			Tags:        []dbmodel.KeyValue{{Key: "num", Value: "16.66", Type: dbmodel.Float64Type}},
		},
	}, spans[0])
}

func TestEmpty(t *testing.T) {
	c := &Translator{}
	spans, err := c.ConvertSpans(pdata.NewTraces())
	require.NoError(t, err)
	assert.Nil(t, spans)
}

func TestErrorIDs(t *testing.T) {
	zero64Bytes := make([]byte, 16)
	binary.LittleEndian.PutUint64(zero64Bytes, 0)
	binary.LittleEndian.PutUint64(zero64Bytes, 0)
	tests := []struct {
		spanID  []byte
		traceID []byte
		err     string
	}{
		{
			traceID: []byte("invalid-%"),
			err:     "TraceID does not have 16 bytes",
		},
		{
			traceID: traceID,
			spanID:  []byte("invalid-%"),
			err:     "SpanID does not have 8 bytes",
		},
		{
			traceID: traceID,
			spanID:  zero64Bytes[:8],
			err:     errZeroSpanID.Error(),
		},
		{
			traceID: zero64Bytes,
			spanID:  spanID,
			err:     errZeroTraceID.Error(),
		},
	}
	for _, test := range tests {
		t.Run(test.err, func(t *testing.T) {
			c := &Translator{}
			traces := traces("foo")
			addSpan(traces, "foo", test.traceID, test.spanID)
			spans, err := c.ConvertSpans(traces)
			assert.EqualError(t, err, test.err)
			assert.Nil(t, spans)
		})
	}

}

func traces(serviceName string) pdata.Traces {
	traces := pdata.NewTraces()
	traces.ResourceSpans().Resize(1)
	traces.ResourceSpans().At(0).InstrumentationLibrarySpans().Resize(1)
	traces.ResourceSpans().At(0).Resource().InitEmpty()
	traces.ResourceSpans().At(0).Resource().Attributes().InitFromMap(map[string]pdata.AttributeValue{conventions.AttributeServiceName: pdata.NewAttributeValueString(serviceName)})
	return traces
}

func addSpan(traces pdata.Traces, name string, traceID []byte, spanID []byte) pdata.Span {
	rspans := traces.ResourceSpans()
	instSpans := rspans.At(0).InstrumentationLibrarySpans()
	spans := instSpans.At(0).Spans()
	spans.Resize(spans.Len() + 1)
	span := spans.At(spans.Len() - 1)
	span.SetName(name)
	span.SetTraceID(traceID)
	span.SetSpanID(spanID)
	span.SetStartTime(pdata.TimestampUnixNano(time.Now().UnixNano()))
	span.SetEndTime(pdata.TimestampUnixNano(time.Now().UnixNano()))
	return span
}
