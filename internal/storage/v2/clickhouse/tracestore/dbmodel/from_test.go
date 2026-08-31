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

	spanRow := createTestSpanRow(t, now, duration)

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
		{
			name: "empty trace id",
			arg: &SpanRow{
				TraceID: "",
			},
			want: `failed to decode trace ID: invalid length 0 of decoded trace ID "", expected 16 bytes`,
		},
		{
			name: "too short trace id",
			arg: &SpanRow{
				TraceID: "0001",
			},
			want: `failed to decode trace ID: invalid length 2 of decoded trace ID "0001", expected 16 bytes`,
		},
		{
			name: "too long trace id",
			arg: &SpanRow{
				TraceID: "000100010001000100010001000100010001",
			},
			want: `failed to decode trace ID: invalid length 18 of decoded trace ID "000100010001000100010001000100010001", expected 16 bytes`,
		},
		{
			name: "empty span id",
			arg: &SpanRow{
				TraceID: "00010001000100010001000100010001",
				ID:      "",
			},
			want: `failed to decode span ID: invalid length 0 of decoded span ID "", expected 8 bytes`,
		},
		{
			name: "too short span id",
			arg: &SpanRow{
				TraceID: "00010001000100010001000100010001",
				ID:      "0001",
			},
			want: `failed to decode span ID: invalid length 2 of decoded span ID "0001", expected 8 bytes`,
		},
		{
			name: "too short parent span id",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001",
			},
			want: `failed to decode parent span ID: invalid length 2 of decoded span ID "0001", expected 8 bytes`,
		},
		{
			name: "empty link trace id",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				LinkTraceIDs: []string{""},
			},
			want: `failed to decode link trace ID: invalid length 0 of decoded trace ID "", expected 16 bytes`,
		},
		{
			name: "too short link trace id",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				LinkTraceIDs: []string{"0001"},
			},
			want: `failed to decode link trace ID: invalid length 2 of decoded trace ID "0001", expected 16 bytes`,
		},
		{
			name: "empty link span id",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				LinkTraceIDs: []string{"00010001000100010001000100010001"},
				LinkSpanIDs:  []string{""},
			},
			want: `failed to decode link span ID: invalid length 0 of decoded span ID "", expected 8 bytes`,
		},
		{
			name: "too short link span id",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				LinkTraceIDs: []string{"00010001000100010001000100010001"},
				LinkSpanIDs:  []string{"0001"},
			},
			want: `failed to decode link span ID: invalid length 2 of decoded span ID "0001", expected 8 bytes`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var trace ptrace.Traces
			require.NotPanics(t, func() {
				trace = FromRow(tt.arg)
			})
			span := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
			require.Contains(t, jptrace.GetWarnings(span), tt.want)
		})
	}
}

func TestFromRow_EmptyParentSpanID(t *testing.T) {
	trace := FromRow(&SpanRow{
		TraceID:      "00010001000100010001000100010001",
		ID:           "0001000100010001",
		ParentSpanID: "",
	})
	span := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.True(t, span.ParentSpanID().IsEmpty())
	require.Empty(t, jptrace.GetWarnings(span))
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
			name:                 "failed to unmarshal slice attribute",
			complexKeys:          []string{"@slice@slice-key"},
			complexValues:        []string{"notjson"},
			expectedWarnContains: "failed to unmarshal slice attribute \"@slice@slice-key\"",
		},
		{
			name:                 "failed to unmarshal map attribute",
			complexKeys:          []string{"@map@map-key"},
			complexValues:        []string{"notjson"},
			expectedWarnContains: "failed to unmarshal map attribute \"@map@map-key\"",
		},
		{
			name:                 "unsupported complex attribute key",
			complexKeys:          []string{"unsupported"},
			complexValues:        []string{"{\"kvlistValue\":{\"values\":[{\"key\":\"key\",\"value\":{\"stringValue\":\"value\"}}]}}"},
			expectedWarnContains: "unsupported complex attribute key: \"unsupported\"",
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
