// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	// The ID parsers live in jptrace and their errors are tested there. This
	// test only checks that FromRow turns each failure into a warning that
	// names the field, and that it never panics on a corrupted row.
	tests := []struct {
		name       string
		arg        *SpanRow
		wantPrefix string
	}{
		{
			name: "decode span trace id failed",
			arg: &SpanRow{
				TraceID: "0x",
			},
			wantPrefix: "failed to decode trace ID: ",
		},
		{
			name: "decode span id failed",
			arg: &SpanRow{
				TraceID: "00010001000100010001000100010001",
				ID:      "0x",
			},
			wantPrefix: "failed to decode span ID: ",
		},
		{
			name: "decode span parent id failed",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0x",
			},
			wantPrefix: "failed to decode parent span ID: ",
		},
		{
			name: "decode link trace id failed",
			arg: &SpanRow{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				LinkTraceIDs: []string{"0x"},
			},
			wantPrefix: "failed to decode link trace ID: ",
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
			wantPrefix: "failed to decode link span ID: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var trace ptrace.Traces
			require.NotPanics(t, func() {
				trace = FromRow(tt.arg)
			})
			span := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
			warnings := jptrace.GetWarnings(span)
			require.Len(t, warnings, 1)
			assert.True(t, strings.HasPrefix(warnings[0], tt.wantPrefix), "warning %q lacks prefix %q", warnings[0], tt.wantPrefix)
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
