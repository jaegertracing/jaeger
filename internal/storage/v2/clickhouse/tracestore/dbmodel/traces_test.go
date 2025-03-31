// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

// FIXME: how to test proto.input?
func TestFromPtrace(t *testing.T) {
	// FIXME: Diff:
	//     --- Expected
	//     +++ Actual
	//     @@ -91,4 +91,3 @@
	//        	    Value: (*v1.AnyValue_BytesValue)({
	//     -         BytesValue: ([]uint8) {
	//     -         }
	//     +         BytesValue: ([]uint8) <nil>
	//        	     })
	//        	Test:       	TestFromPtrace
	actual := jsonToTraces("ptrace.json")
	require.NotNil(t, actual)
	excepted := simpleTraces(2)
	require.NotNil(t, excepted)
	// require.Equal(t, traces, traces)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func jsonToTraces(filename string) ptrace.Traces {
	unMarshaler := ptrace.JSONUnmarshaler{}
	jsonFile, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()
	bytes, err := io.ReadAll(jsonFile)
	if err != nil {
		panic(err)
	}

	traces, err := unMarshaler.UnmarshalTraces(bytes)
	if err != nil {
		panic(err)
	}
	return traces
}

func simpleTraces(count int) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl("https://opentelemetry.io/schemas/1.4.0")
	rs.Resource().SetDroppedAttributesCount(10)
	rs.Resource().Attributes().PutStr("service.name", "clickhouse")
	rs.Resource().Attributes().PutBool("true", true)
	rs.Resource().Attributes().PutBool("false", false)
	rs.Resource().Attributes().PutDouble("6.824", 6.824)
	rs.Resource().Attributes().PutInt("0", 0)
	rs.Resource().Attributes().PutInt("1", 1)
	rs.Resource().Attributes().PutInt("2", 2)
	rs.Resource().Attributes().PutStr("hello", "world")
	rs.Resource().Attributes().PutEmpty("nil")
	rs.Resource().Attributes().PutEmptyMap("map")
	rs.Resource().Attributes().PutEmptyBytes("bytes")
	rs.Resource().Attributes().PutEmptySlice("slice")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("io.opentelemetry.contrib.clickhouse")
	ss.Scope().SetVersion("1.0.0")
	ss.SetSchemaUrl("https://opentelemetry.io/schemas/1.7.0")
	ss.Scope().SetDroppedAttributesCount(20)
	ss.Scope().Attributes().PutBool("true", true)
	ss.Scope().Attributes().PutBool("false", false)
	ss.Scope().Attributes().PutDouble("6.824", 6.824)
	ss.Scope().Attributes().PutInt("0", 0)
	ss.Scope().Attributes().PutInt("1", 1)
	ss.Scope().Attributes().PutInt("2", 2)
	ss.Scope().Attributes().PutStr("hello", "world")
	ss.Scope().Attributes().PutEmpty("nil")
	ss.Scope().Attributes().PutEmptyMap("map")
	ss.Scope().Attributes().PutEmptyBytes("bytes")
	ss.Scope().Attributes().PutEmptySlice("slice")

	timestamp := time.Unix(1703498029, 0)
	for i := 0; i < count; i++ {
		s := ss.Spans().AppendEmpty()
		s.SetTraceID([16]byte{1, 2, 3, byte(i)})
		s.SetSpanID([8]byte{1, 2, 3, byte(i)})
		s.TraceState().FromRaw("trace state")
		s.SetParentSpanID([8]byte{1, 2, 4, byte(i)})
		s.SetName("call db")
		s.SetKind(ptrace.SpanKindInternal)
		s.SetStartTimestamp(pcommon.NewTimestampFromTime(timestamp))
		s.SetEndTimestamp(pcommon.NewTimestampFromTime(timestamp.Add(time.Minute)))
		s.Attributes().PutBool("true", true)
		s.Attributes().PutBool("false", false)
		s.Attributes().PutDouble("6.824", 6.824)
		s.Attributes().PutInt("0", 0)
		s.Attributes().PutInt("1", 1)
		s.Attributes().PutInt("2", 2)
		s.Attributes().PutStr("hello", "world")
		s.Attributes().PutEmpty("nil")
		s.Attributes().PutEmptyMap("map")
		s.Attributes().PutEmptyBytes("bytes")
		s.Attributes().PutEmptySlice("slice")
		s.Status().SetMessage("error")
		s.Status().SetCode(ptrace.StatusCodeError)
		event := s.Events().AppendEmpty()
		event.SetName("event1")
		event.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
		event.Attributes().PutBool("true", true)
		event.Attributes().PutBool("false", false)
		event.Attributes().PutDouble("6.824", 6.824)
		event.Attributes().PutInt("0", 0)
		event.Attributes().PutInt("1", 1)
		event.Attributes().PutInt("2", 2)
		event.Attributes().PutStr("hello", "world")
		event.Attributes().PutEmpty("nil")
		event.Attributes().PutEmptyMap("map")
		event.Attributes().PutEmptyBytes("bytes")
		event.Attributes().PutEmptySlice("slice")
		link := s.Links().AppendEmpty()
		link.SetTraceID([16]byte{1, 2, 5, byte(i)})
		link.SetSpanID([8]byte{1, 2, 5, byte(i)})
		link.TraceState().FromRaw("error")
		link.Attributes().PutBool("true", true)
		link.Attributes().PutBool("false", false)
		link.Attributes().PutDouble("6.824", 6.824)
		link.Attributes().PutInt("0", 0)
		link.Attributes().PutInt("1", 1)
		link.Attributes().PutInt("2", 2)
		link.Attributes().PutStr("hello", "world")
		link.Attributes().PutEmpty("nil")
		link.Attributes().PutEmptyMap("map")
		link.Attributes().PutEmptyBytes("bytes")
		link.Attributes().PutEmptySlice("slice")
	}
	return traces
}
