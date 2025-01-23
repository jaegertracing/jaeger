// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/model"
)

func TestToOTELTraceID(t *testing.T) {
	modelTraceID := model.TraceID{
		Low:  3,
		High: 2,
	}
	otelTraceID := model.ToOTELTraceID(modelTraceID)
	expected := []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3}
	require.Equal(t, pcommon.TraceID(expected), otelTraceID)
}

func TestTraceIDFromOTEL(t *testing.T) {
	otelTraceID := pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3})
	expected := model.TraceID{
		Low:  3,
		High: 2,
	}
	require.Equal(t, expected, model.TraceIDFromOTEL(otelTraceID))
}

func TestToOTELSpanID(t *testing.T) {
	tests := []struct {
		name     string
		spanID   model.SpanID
		expected pcommon.SpanID
	}{
		{
			name:     "zero span ID",
			spanID:   model.NewSpanID(0),
			expected: pcommon.NewSpanIDEmpty(),
		},
		{
			name:     "non-zero span ID",
			spanID:   model.NewSpanID(1),
			expected: pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := model.ToOTELSpanID(test.spanID)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestSpanIDFromOTEL(t *testing.T) {
	tests := []struct {
		name       string
		otelSpanID pcommon.SpanID
		expected   model.SpanID
	}{
		{
			name:       "zero span ID",
			otelSpanID: pcommon.NewSpanIDEmpty(),
			expected:   model.NewSpanID(0),
		},
		{
			name:       "non-zero span ID",
			otelSpanID: pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}),
			expected:   model.NewSpanID(1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := model.SpanIDFromOTEL(test.otelSpanID)
			assert.Equal(t, test.expected, actual)
		})
	}
}
