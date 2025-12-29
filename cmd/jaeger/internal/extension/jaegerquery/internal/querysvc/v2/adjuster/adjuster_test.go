// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/adjuster"
)

type mockAdjuster struct{}

func (mockAdjuster) Adjust(traces ptrace.Traces) {
	span := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	spanId := span.SpanID()
	spanId[7]++
	span.SetSpanID(spanId)
}

func TestSequences(t *testing.T) {
	trace := ptrace.NewTraces()
	span := trace.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 0})

	a := adjuster.Sequence(mockAdjuster{}, mockAdjuster{})

	a.Adjust(trace)

	adjTraceSpan := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, span, adjTraceSpan)
	assert.EqualValues(t, [8]byte{0, 0, 0, 0, 0, 0, 0, 2}, span.SpanID())
}
