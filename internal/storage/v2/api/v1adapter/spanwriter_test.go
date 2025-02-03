// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	tracestoremocks "github.com/jaegertracing/jaeger/storage_v2/tracestore/mocks"
)

func TestSpanWriter_WriteSpan(t *testing.T) {
	tests := []struct {
		name          string
		mockReturnErr error
		expectedErr   error
	}{
		{
			name:          "success",
			mockReturnErr: nil,
			expectedErr:   nil,
		},
		{
			name:          "error in translator",
			mockReturnErr: assert.AnError,
			expectedErr:   assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockTraceWriter := &tracestoremocks.Writer{}
			spanWriter := NewSpanWriter(mockTraceWriter)

			now := time.Now().UTC()
			testSpan := &model.Span{
				TraceID:   model.NewTraceID(0, 1),
				SpanID:    model.NewSpanID(1),
				StartTime: now,
				Duration:  time.Second,
			}

			traces := ptrace.NewTraces()
			resources := traces.ResourceSpans().AppendEmpty()
			scopes := resources.ScopeSpans().AppendEmpty()
			span := scopes.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
			span.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
			span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Second)))

			mockTraceWriter.On("WriteTraces", mock.Anything, traces).Return(test.mockReturnErr)

			err := spanWriter.WriteSpan(context.Background(), testSpan)
			require.ErrorIs(t, err, test.expectedErr)
		})
	}
}
