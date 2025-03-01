// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.27.0"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

var expected = "INSERT INTO \"otel_traces\" (\"Timestamp\",\"TraceId\",\"SpanId\",\"ParentSpanId\",\"TraceState\"," +
	"\"SpanName\",\"SpanKind\",\"ServiceName\",\"ResourceAttributes.keys\",\"ResourceAttributes.values\",\"ScopeName\"," +
	"\"ScopeVersion\",\"SpanAttributes.keys\",\"SpanAttributes.values\",\"Duration\",\"StatusCode\",\"StatusMessage\"," +
	"\"Events.Timestamp\",\"Events.Name\",\"Events.Attributes\",\"Links.TraceId\",\"Links.SpanId\",\"Links.TraceState\"," +
	"\"Links.Attributes\") VALUES"

func TestInput(t *testing.T) {
	td := simpleTraces(2)
	actual := Input(td)
	assert.Equal(t, expected, actual.Into("otel_traces"))
}

func simpleTraces(count int) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl("https://opentelemetry.io/schemas/1.4.0")
	rs.Resource().SetDroppedAttributesCount(10)
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("io.opentelemetry.contrib.clickhouse")
	ss.Scope().SetVersion("1.0.0")
	ss.SetSchemaUrl("https://opentelemetry.io/schemas/1.7.0")
	ss.Scope().SetDroppedAttributesCount(20)
	ss.Scope().Attributes().PutStr("lib", "clickhouse")
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
		s.Attributes().PutStr(conventions.AttributeServiceName, "v")
		s.Status().SetMessage("error")
		s.Status().SetCode(ptrace.StatusCodeError)
		event := s.Events().AppendEmpty()
		event.SetName("event1")
		event.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
		event.Attributes().PutStr("level", "info")
		link := s.Links().AppendEmpty()
		link.SetTraceID([16]byte{1, 2, 5, byte(i)})
		link.SetSpanID([8]byte{1, 2, 5, byte(i)})
		link.TraceState().FromRaw("error")
		link.Attributes().PutStr("k", "v")
	}
	return traces
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
