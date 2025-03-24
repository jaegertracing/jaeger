// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestConvertToTraces(t *testing.T) {
	tests := []struct {
		name         string
		model        Model
		expectsError bool
		errorMsg     string
	}{
		{
			name:         "should return error when traceId is invalid",
			model:        Model{TraceId: "0xff"},
			expectsError: true,
			errorMsg:     "encoding/hex: invalid byte:",
		},
		{
			name:         "should return error when traceId is empty",
			model:        Model{TraceId: "00000000000000000000000000000000"},
			expectsError: true,
			errorMsg:     "trace id is empty",
		},
		{
			name:         "should return error when spanId is empty",
			model:        Model{TraceId: "00010001000100010001000100010001", SpanId: "0000000000000000"},
			expectsError: true,
			errorMsg:     "span id is empty",
		},
		{
			name:         "should return error when spanId is invalid",
			model:        Model{TraceId: "00010001000100010001000100010001", SpanId: "0xff"},
			expectsError: true,
			errorMsg:     "encoding/hex: invalid byte:",
		},
		{
			name:         "should return error when parentSpanId is empty",
			model:        Model{TraceId: "00010001000100010001000100010001", SpanId: "0000000000000001", ParentSpanId: "00000000"},
			expectsError: true,
			errorMsg:     "parentSpan id is empty",
		},
		{
			name:         "should return error when parentSpanId is invalid",
			model:        Model{TraceId: "00010001000100010001000100010001", SpanId: "0000000000000001", ParentSpanId: "0xff"},
			expectsError: true,
			errorMsg:     "encoding/hex: invalid byte:",
		},
		{
			name: "should return error when convertLink fails",
			model: Model{
				TraceId:              "00010001000100010001000100010001",
				SpanId:               "0000000000000001",
				ParentSpanId:         "0000000000000001",
				SpanAttributesKeys:   []string{"hello"},
				SpanAttributesValues: []string{"world"},
				LinksTraceId:         []string{"0xff"},
				LinksSpanId:          []string{"0000000000000001"},
				LinksTraceState:      []string{"good"},
				EventsName:           []string{"even_name"},
				EventsTimestamp:      []time.Time{time.Now()},
				EventsAttributes:     []map[string]string{{"event_key": "event_val"}},
			},
			expectsError: true,
			errorMsg:     "encoding/hex: invalid byte:",
		},
		{
			name: "should successfully convert model to traces when data is valid",
			model: Model{
				TraceId:              "00010001000100010001000100010001",
				SpanId:               "0000000000000001",
				ParentSpanId:         "0000000000000001",
				SpanAttributesKeys:   []string{"hello"},
				SpanAttributesValues: []string{"world"},
				LinksTraceId:         []string{"00010001000100010001000100010001"},
				LinksSpanId:          []string{"0000000000000001"},
				LinksTraceState:      []string{"good"},
				EventsName:           []string{"even_name"},
				EventsAttributes:     []map[string]string{{"event_key": "event_val"}},
			},
			expectsError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt, err := tt.model.ConvertToTraces()
			if tt.expectsError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pt)
			}
		})
	}
}

func TestConvertLink(t *testing.T) {
	tests := []struct {
		name          string
		model         Model
		expectedError bool
		errorMsg      string
	}{
		{
			name: "should return error when linksTraceId and linksSpanId are empty",
			model: Model{
				LinksTraceId: []string{""},
				LinksSpanId:  []string{""},
			},
			expectedError: true,
			errorMsg:      "invalid trace id or span id",
		},
		{
			name: "should return error when linksTraceId is invalid",
			model: Model{
				LinksTraceId: []string{"invalidTraceId"},
				LinksSpanId:  []string{"0x1"},
			},
			expectedError: true,
		},
		{
			name: "should return error when linksTraceId is invalid byte",
			model: Model{
				LinksTraceId: []string{"invalidTraceId"},
				LinksSpanId:  []string{"0xff"},
			},
			expectedError: true,
			errorMsg:      "encoding/hex: invalid byte:",
		},
		{
			name: "should return error when linksSpanId is invalid",
			model: Model{
				LinksTraceId: []string{"00010001000100010001000100010001"},
				LinksSpanId:  []string{"invalidSpanId"},
			},
			expectedError: true,
		},
		{
			name: "should return error when linksSpanId is invalid byte",
			model: Model{
				LinksTraceId: []string{"00010001000100010001000100010001"},
				LinksSpanId:  []string{"0xff"},
			},
			expectedError: true,
			errorMsg:      "encoding/hex: invalid byte:",
		},
		{
			name: "should successfully convert link when valid data is provided",
			model: Model{
				LinksTraceId:     []string{"00010001000100010001000100010001"},
				LinksSpanId:      []string{"0000000000000001"},
				LinksTraceState:  []string{"ha?"},
				LinksAttributes:  []map[string]string{{"link_key": "link_val"}},
				EventsName:       []string{"event"},
				EventsTimestamp:  []time.Time{time.Now()},
				EventsAttributes: []map[string]string{{"event_key": "event_val"}},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := ptrace.NewSpanLink()
			err := tt.model.convertLink(links, 0)
			if tt.expectedError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStatusCode(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    string
		expectedValue ptrace.StatusCode
	}{
		{
			name:          "should return StatusCodeError when status code is 'Error'",
			statusCode:    "Error",
			expectedValue: ptrace.StatusCodeError,
		},
		{
			name:          "should return StatusCodeUnset when status code is 'Unset'",
			statusCode:    "Unset",
			expectedValue: ptrace.StatusCodeUnset,
		},
		{
			name:          "should return StatusCodeOk when status code is 'Ok'",
			statusCode:    "Ok",
			expectedValue: ptrace.StatusCodeOk,
		},
		{
			name:          "should return invalid status code (-1) when status code is unknown",
			statusCode:    "unknown",
			expectedValue: ptrace.StatusCode(-1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := Model{StatusCode: tt.statusCode}
			assert.Equal(t, tt.expectedValue, model.statusCode())
		})
	}
}

func TestSpanKind(t *testing.T) {
	tests := []struct {
		name          string
		spanKind      string
		expectedValue ptrace.SpanKind
	}{
		{
			name:          "should return SpanKindUnspecified when span kind is 'UNSPECIFIED'",
			spanKind:      "UNSPECIFIED",
			expectedValue: ptrace.SpanKindUnspecified,
		},
		{
			name:          "should return SpanKindInternal when span kind is 'Internal'",
			spanKind:      "Internal",
			expectedValue: ptrace.SpanKindInternal,
		},
		{
			name:          "should return SpanKindServer when span kind is 'Service'",
			spanKind:      "Service",
			expectedValue: ptrace.SpanKindServer,
		},
		{
			name:          "should return SpanKindClient when span kind is 'Client'",
			spanKind:      "Client",
			expectedValue: ptrace.SpanKindClient,
		},
		{
			name:          "should return SpanKindProducer when span kind is 'Producer'",
			spanKind:      "Producer",
			expectedValue: ptrace.SpanKindProducer,
		},
		{
			name:          "should return SpanKindConsumer when span kind is 'Consumer'",
			spanKind:      "Consumer",
			expectedValue: ptrace.SpanKindConsumer,
		},
		{
			name:          "should return invalid span kind (-1) when span kind is unknown",
			spanKind:      "Unknown",
			expectedValue: ptrace.SpanKind(-1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := Model{SpanKind: tt.spanKind}
			assert.Equal(t, tt.expectedValue, model.spanKind())
		})
	}
}

func TestIsZeroBytes(t *testing.T) {
	var b []byte
	assert.True(t, isZeroBytes(b))
}
