// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func TestFromRow(t *testing.T) {
	now := time.Now().UTC()
	duration := 2 * time.Second

	spanRow := createTestSpanRow(now, duration)

	expected := createTestTrace(now, duration)

	row := FromRow(spanRow)
	require.Equal(t, expected, row)
}

func TestFromRow_DecodeID(t *testing.T) {
	tests := []struct {
		name string
		arg  *SpanRow
		want string
	}{
		{
			name: "decode span trace id failed",
			arg: &SpanRow{
				TraceID: "0x",
			},
			want: "failed to decode trace ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode span id failed",
			arg: &SpanRow{
				TraceID: "00010001000100010001000100010001",
				ID:      "0x",
			},
			want: "failed to decode span ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode span parent id failed",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0x",
			},
			want: "failed to decode parent span ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode link trace id failed",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				LinkTraceIDs: []string{"0x"},
			},
			want: "failed to decode link trace ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode link span id failed",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				LinkTraceIDs: []string{"00010001000100010001000100010001"},
				LinkSpanIDs:  []string{"0x"},
			},
			want: "failed to decode link span ID: encoding/hex: invalid byte: U+0078 'x'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace := FromRow(tt.arg)
			span := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
			require.Contains(t, jptrace.GetWarnings(span), tt.want)
		})
	}
}

func TestPutAttributes_Warnings(t *testing.T) {
	tests := []struct {
		name                 string
		complexKeys          []string
		complexValues        []string
		expectedWarnContains string
	}{
		{
			name:                 "bytes attribute with invalid base64",
			complexKeys:          []string{"@bytes@bytes-key"},
			complexValues:        []string{"invalid-base64"},
			expectedWarnContains: "failed to decode bytes attribute \"@bytes@bytes-key\"",
		},
		{
			name:                 "slice attribute with invalid base64",
			complexKeys:          []string{"@slice@slice-key"},
			complexValues:        []string{"invalid-base64"},
			expectedWarnContains: "failed to decode slice attribute \"@slice@slice-key\"",
		},
		{
			name:                 "failed to unmarshal slice attribute",
			complexKeys:          []string{"@slice@slice-key"},
			complexValues:        []string{"dGVzdA=="},
			expectedWarnContains: "failed to unmarshal slice attribute \"@slice@slice-key\"",
		},
		{
			name:                 "map attribute with invalid base64",
			complexKeys:          []string{"@map@map-key"},
			complexValues:        []string{"invalid-base64"},
			expectedWarnContains: "failed to decode map attribute \"@map@map-key\"",
		},
		{
			name:                 "failed to unmarshal map attribute",
			complexKeys:          []string{"@map@map-key"},
			complexValues:        []string{"dGVzdA=="},
			expectedWarnContains: "failed to unmarshal map attribute \"@map@map-key\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			attributes := pcommon.NewMap()

			putAttributes(
				attributes,
				&Attributes{
					ComplexKeys:   tt.complexKeys,
					ComplexValues: tt.complexValues,
				},
				span,
			)

			warnings := jptrace.GetWarnings(span)
			require.Len(t, warnings, 1)
			require.Contains(t, warnings[0], tt.expectedWarnContains)
		})
	}
}
