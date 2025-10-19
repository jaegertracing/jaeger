// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"github.com/stretchr/testify/require"
)

func TestToRow(t *testing.T) {
	now := time.Now().UTC()
	duration := 2 * time.Second

	rs := createTestResource()
	sc := createTestScope()
	span := createTestSpan(now, duration)

	expected := createExpectedSpanRow(now, duration)

	row := ToRow(rs, sc, span)
	require.Equal(t, expected, row)
}

func createTestResource() pcommon.Resource {
	rs := pcommon.NewResource()
	rs.Attributes().PutStr(otelsemconv.ServiceNameKey, "test-service")
	return rs
}

func createTestScope() pcommon.InstrumentationScope {
	sc := pcommon.NewInstrumentationScope()
	sc.SetName("test-scope")
	sc.SetVersion("v1.0.0")
	return sc
}

func createTestSpan(now time.Time, duration time.Duration) ptrace.Span {
	span := ptrace.NewSpan()
	span.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	span.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
	span.TraceState().FromRaw("state1")
	span.SetParentSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	span.SetName("test-span")
	span.SetKind(ptrace.SpanKindServer)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(duration)))
	span.Status().SetCode(ptrace.StatusCodeOk)
	span.Status().SetMessage("test-status-message")

	addSpanAttributes(span)
	addSpanEvent(span, now)
	addSpanLink(span)

	return span
}

func addSpanAttributes(span ptrace.Span) {
	attrs := span.Attributes()
	attrs.PutStr("string_attr", "string_value")
	attrs.PutInt("int_attr", 42)
	attrs.PutDouble("double_attr", 3.14)
	attrs.PutBool("bool_attr", true)
	attrs.PutEmptyBytes("bytes_attr").FromRaw([]byte("bytes_value"))
}

func addSpanEvent(span ptrace.Span, now time.Time) {
	event := span.Events().AppendEmpty()
	event.SetName("test-event")
	event.SetTimestamp(pcommon.NewTimestampFromTime(now))
	addTestAttributes(event.Attributes())
}

func addSpanLink(span ptrace.Span) {
	link := span.Links().AppendEmpty()
	link.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3}))
	link.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 4}))
	link.TraceState().FromRaw("link-state")
	addTestAttributes(link.Attributes())
}

func addTestAttributes(attrs pcommon.Map) {
	attrs.PutStr("string_attr", "string_value")
	attrs.PutInt("int_attr", 42)
	attrs.PutDouble("double_attr", 3.14)
	attrs.PutBool("bool_attr", true)
	attrs.PutEmptyBytes("bytes_attr").FromRaw([]byte("bytes_value"))
}

func createExpectedSpanRow(now time.Time, duration time.Duration) *SpanRow {
	encodedBytes := base64.StdEncoding.EncodeToString([]byte("bytes_value"))
	return &SpanRow{
		ID:                          "0000000000000001",
		TraceID:                     "00000000000000000000000000000001",
		TraceState:                  "state1",
		ParentSpanID:                "0000000000000002",
		Name:                        "test-span",
		Kind:                        "server",
		StartTime:                   now,
		StatusCode:                  "Ok",
		StatusMessage:               "test-status-message",
		Duration:                    duration.Nanoseconds(),
		StrAttributeKeys:            []string{"string_attr"},
		StrAttributeValues:          []string{"string_value"},
		IntAttributeKeys:            []string{"int_attr"},
		IntAttributeValues:          []int64{42},
		DoubleAttributeKeys:         []string{"double_attr"},
		DoubleAttributeValues:       []float64{3.14},
		BoolAttributeKeys:           []string{"bool_attr"},
		BoolAttributeValues:         []bool{true},
		ComplexAttributeKeys:        []string{"@bytes@bytes_attr"},
		ComplexAttributeValues:      []string{encodedBytes},
		EventNames:                  []string{"test-event"},
		EventTimestamps:             []time.Time{now},
		EventStrAttributeKeys:       [][]string{{"string_attr"}},
		EventStrAttributeValues:     [][]string{{"string_value"}},
		EventIntAttributeKeys:       [][]string{{"int_attr"}},
		EventIntAttributeValues:     [][]int64{{42}},
		EventDoubleAttributeKeys:    [][]string{{"double_attr"}},
		EventDoubleAttributeValues:  [][]float64{{3.14}},
		EventBoolAttributeKeys:      [][]string{{"bool_attr"}},
		EventBoolAttributeValues:    [][]bool{{true}},
		EventComplexAttributeKeys:   [][]string{{"@bytes@bytes_attr"}},
		EventComplexAttributeValues: [][]string{{encodedBytes}},
		LinkTraceIDs:                []string{"00000000000000000000000000000003"},
		LinkSpanIDs:                 []string{"0000000000000004"},
		LinkTraceStates:             []string{"link-state"},
		LinkStrAttributeKeys:        [][]string{{"string_attr"}},
		LinkStrAttributeValues:      [][]string{{"string_value"}},
		LinkIntAttributeKeys:        [][]string{{"int_attr"}},
		LinkIntAttributeValues:      [][]int64{{42}},
		LinkDoubleAttributeKeys:     [][]string{{"double_attr"}},
		LinkDoubleAttributeValues:   [][]float64{{3.14}},
		LinkBoolAttributeKeys:       [][]string{{"bool_attr"}},
		LinkBoolAttributeValues:     [][]bool{{true}},
		LinkComplexAttributeKeys:    [][]string{{"@bytes@bytes_attr"}},
		LinkComplexAttributeValues:  [][]string{{encodedBytes}},
		ServiceName:                 "test-service",
		ScopeName:                   "test-scope",
		ScopeVersion:                "v1.0.0",
	}
}
