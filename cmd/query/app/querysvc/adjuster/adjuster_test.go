// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc/adjuster"
)

type mockAdjuster struct{}

func (mockAdjuster) Adjust(traces ptrace.Traces) error {
	span := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	spanId := span.SpanID()
	spanId[7]++
	span.SetSpanID(spanId)
	return nil
}

type mockAdjusterError struct{}

func (mockAdjusterError) Adjust(ptrace.Traces) error {
	return assert.AnError
}

func TestSequences(t *testing.T) {
	tests := []struct {
		name       string
		adjuster   adjuster.Adjuster
		err        string
		lastSpanID pcommon.SpanID
	}{
		{
			name:       "normal sequence",
			adjuster:   adjuster.Sequence(mockAdjuster{}, mockAdjusterError{}, mockAdjuster{}, mockAdjusterError{}),
			err:        fmt.Sprintf("%s\n%s", assert.AnError, assert.AnError),
			lastSpanID: [8]byte{0, 0, 0, 0, 0, 0, 0, 2},
		},
		{
			name:       "fail fast sequence",
			adjuster:   adjuster.FailFastSequence(mockAdjuster{}, mockAdjusterError{}, mockAdjuster{}, mockAdjusterError{}),
			err:        assert.AnError.Error(),
			lastSpanID: [8]byte{0, 0, 0, 0, 0, 0, 0, 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace := ptrace.NewTraces()
			span := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 0})

			err := test.adjuster.Adjust(trace)
			require.EqualError(t, err, test.err)

			adjTraceSpan := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
			assert.Equal(t, span, adjTraceSpan)
			assert.EqualValues(t, test.lastSpanID, span.SpanID())
		})
	}
}
