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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.opentelemetry.io/collector/translator/conventions"
	tracetranslator "go.opentelemetry.io/collector/translator/trace"

	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

var (
	traceID = pdata.NewTraceID([16]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F})
	spanID = pdata.NewSpanID([8]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
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
	instrumentationLibrary := traces.ResourceSpans().At(0).InstrumentationLibrarySpans().At(0).InstrumentationLibrary()
	instrumentationLibrary.SetName("io.opentelemetry")
	instrumentationLibrary.SetVersion("1.0")
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
	span.Links().At(0).SetSpanID(spanID)
	traceIDZeroHigh := pdata.NewTraceID([16]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	span.Links().At(0).SetTraceID(traceIDZeroHigh)

	c := &Translator{
		tagKeysAsFields: map[string]bool{"toTagMap": true},
	}
	spansData, err := c.ConvertSpans(traces)
	require.NoError(t, err)
	assert.Equal(t, 1, len(spansData))
	assert.Equal(t,
		ConvertedData{
			Span:                   span,
			Resource:               resource,
			InstrumentationLibrary: traces.ResourceSpans().At(0).InstrumentationLibrarySpans().At(0).InstrumentationLibrary(),
			DBSpan: &dbmodel.Span{
				TraceID:         "000102030405060708090a0b0c0d0e0f",
				SpanID:          "0001020304050607",
				StartTime:       1000,
				Duration:        1000,
				OperationName:   "root",
				StartTimeMillis: 1,
				Tags: []dbmodel.KeyValue{
					{Key: "span.kind", Type: dbmodel.StringType, Value: "client"},
					{Key: "status.code", Type: dbmodel.StringType, Value: "STATUS_CODE_OK"},
					{Key: "error", Type: dbmodel.BoolType, Value: "true"},
					{Key: "status.message", Type: dbmodel.StringType, Value: "messagetext"},
					{Key: "foo", Type: dbmodel.BoolType, Value: "true"},
					{Key: tracetranslator.TagInstrumentationName, Type: dbmodel.StringType, Value: "io.opentelemetry"},
					{Key: tracetranslator.TagInstrumentationVersion, Type: dbmodel.StringType, Value: "1.0"},
				},
				Tag: map[string]interface{}{"toTagMap": "val"},
				Logs: []dbmodel.Log{{Fields: []dbmodel.KeyValue{
					{Key: "event", Value: "eventName", Type: dbmodel.StringType},
					{Key: "foo", Value: "bar", Type: dbmodel.StringType}}, Timestamp: 500}},
				References: []dbmodel.Reference{
					{SpanID: "0001020304050607", TraceID: "000102030405060708090a0b0c0d0e0f", RefType: dbmodel.ChildOf},
					{SpanID: "0001020304050607", TraceID: "0001020304050607", RefType: dbmodel.FollowsFrom}},
				Process: dbmodel.Process{
					ServiceName: "myservice",
					Tags:        []dbmodel.KeyValue{{Key: "num", Value: "16.66", Type: dbmodel.Float64Type}},
				},
			},
		}, spansData[0])
}

func BenchmarkConvertSpanID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = convertSpanID(spanID)
	}
}

func TestSpanEmptyRef(t *testing.T) {
	traces := traces("myservice")
	span := addSpan(traces, "root", traceID, spanID)
	span.SetStartTime(pdata.TimestampUnixNano(1000000))
	span.SetEndTime(pdata.TimestampUnixNano(2000000))

	c := &Translator{}
	spansData, err := c.ConvertSpans(traces)
	require.NoError(t, err)
	assert.Equal(t, 1, len(spansData))
	assert.Equal(t,
		ConvertedData{
			Span:                   span,
			Resource:               traces.ResourceSpans().At(0).Resource(),
			InstrumentationLibrary: traces.ResourceSpans().At(0).InstrumentationLibrarySpans().At(0).InstrumentationLibrary(),
			DBSpan: &dbmodel.Span{
				TraceID:         "000102030405060708090a0b0c0d0e0f",
				SpanID:          "0001020304050607",
				StartTime:       1000,
				Duration:        1000,
				OperationName:   "root",
				StartTimeMillis: 1,
				Tags:            []dbmodel.KeyValue{},  // should not be nil
				Logs:            []dbmodel.Log{},       // should not be nil
				References:      []dbmodel.Reference{}, // should not be nil
				Process: dbmodel.Process{
					ServiceName: "myservice",
					Tags:        nil,
				},
			},
		}, spansData[0])
}

func TestEmpty(t *testing.T) {
	c := &Translator{}
	spans, err := c.ConvertSpans(pdata.NewTraces())
	require.NoError(t, err)
	assert.Nil(t, spans)
}

func TestErrorIDs(t *testing.T) {
	var zero64Bytes [16]byte
	var zero32Bytes [8]byte
	tests := []struct {
		spanID  pdata.SpanID
		traceID pdata.TraceID
		err     string
	}{
		{
			traceID: traceID,
			spanID:  pdata.NewSpanID(zero32Bytes),
			err:     errZeroSpanID.Error(),
		},
		{
			traceID: pdata.NewTraceID(zero64Bytes),
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
	traces.ResourceSpans().At(0).Resource().Attributes().InitFromMap(map[string]pdata.AttributeValue{conventions.AttributeServiceName: pdata.NewAttributeValueString(serviceName)})
	return traces
}

func addSpan(traces pdata.Traces, name string, traceID pdata.TraceID, spanID pdata.SpanID) pdata.Span {
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
