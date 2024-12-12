// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc/adjuster"
)

func TestSequences(t *testing.T) {
	// mock adjuster that increments last byte of span ID
	adj := adjuster.Func(func(trace ptrace.Traces) (ptrace.Traces, error) {
		span := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
		spanId := span.SpanID()
		spanId[7]++
		span.SetSpanID(spanId)
		return trace, nil
	})

	adjErr := errors.New("mock adjuster error")
	failingAdj := adjuster.Func(func(trace ptrace.Traces) (ptrace.Traces, error) {
		return trace, adjErr
	})

	tests := []struct {
		name       string
		adjuster   adjuster.Adjuster
		err        string
		lastSpanID pcommon.SpanID
	}{
		{
			name:       "normal sequence",
			adjuster:   adjuster.Sequence(adj, failingAdj, adj, failingAdj),
			err:        fmt.Sprintf("%s\n%s", adjErr, adjErr),
			lastSpanID: [8]byte{0, 0, 0, 0, 0, 0, 0, 2},
		},
		{
			name:       "fail fast sequence",
			adjuster:   adjuster.FailFastSequence(adj, failingAdj, adj, failingAdj),
			err:        adjErr.Error(),
			lastSpanID: [8]byte{0, 0, 0, 0, 0, 0, 0, 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace := ptrace.NewTraces()
			span := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 0})

			adjTrace, err := test.adjuster.Adjust(trace)
			adjTraceSpan := adjTrace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)

			assert.Equal(t, span, adjTraceSpan)
			assert.EqualValues(t, test.lastSpanID, span.SpanID())
			require.EqualError(t, err, test.err)
		})
	}
}
