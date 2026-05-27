// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/adjuster"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

var noopAdjuster = adjuster.Sequence()

func makeMultiServiceTrace() ptrace.Traces {
	td := ptrace.NewTraces()

	// service "frontend" — root span (no parent) + one error span
	fe := td.ResourceSpans().AppendEmpty()
	fe.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "frontend")
	feScope := fe.ScopeSpans().AppendEmpty()

	root := feScope.Spans().AppendEmpty()
	root.SetTraceID(testTraceID)
	root.SetSpanID(pcommon.SpanID([8]byte{1}))
	// ParentSpanID is zero → root span
	root.SetName("HTTP GET /")
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(1000, 0)))
	root.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(1001, 0)))

	errSpan := feScope.Spans().AppendEmpty()
	errSpan.SetTraceID(testTraceID)
	errSpan.SetSpanID(pcommon.SpanID([8]byte{2}))
	errSpan.SetParentSpanID(pcommon.SpanID([8]byte{1}))
	errSpan.Status().SetCode(ptrace.StatusCodeError)
	errSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(1000, 500)))
	errSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(1000, 800)))

	// service "backend" — one normal span
	be := td.ResourceSpans().AppendEmpty()
	be.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "backend")
	beScope := be.ScopeSpans().AppendEmpty()

	child := beScope.Spans().AppendEmpty()
	child.SetTraceID(testTraceID)
	child.SetSpanID(pcommon.SpanID([8]byte{3}))
	child.SetParentSpanID(pcommon.SpanID([8]byte{1}))
	child.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(1000, 100)))
	child.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(1000, 900)))

	return td
}

func TestComputeSummaries_Empty(t *testing.T) {
	summaries, err := jiter.FlattenWithErrors(computeSummaries(func(_ func([]ptrace.Traces, error) bool) {}, noopAdjuster))
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestComputeSummaries_Error(t *testing.T) {
	seq := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield(nil, assert.AnError)
	})
	summaries, err := jiter.FlattenWithErrors(computeSummaries(seq, noopAdjuster))
	require.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, summaries)
}

func TestComputeSummaries_MultiService(t *testing.T) {
	seq := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{makeMultiServiceTrace()}, nil)
	})
	summaries, err := jiter.FlattenWithErrors(computeSummaries(seq, noopAdjuster))
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	s := summaries[0]
	assert.Equal(t, testTraceID, s.TraceID)
	assert.Equal(t, "frontend", s.RootServiceName)
	assert.Equal(t, "HTTP GET /", s.RootOperationName)
	assert.Equal(t, time.Unix(1000, 0).UTC(), s.MinStartTime)
	assert.Equal(t, time.Unix(1001, 0).UTC(), s.MaxEndTime)
	assert.Equal(t, 3, s.SpanCount)
	assert.Equal(t, 1, s.ErrorSpanCount)
	assert.Equal(t, 0, s.OrphanSpanCount)

	// Services must be sorted by name.
	require.Len(t, s.Services, 2)
	assert.Equal(t, "backend", s.Services[0].Name)
	assert.Equal(t, "frontend", s.Services[1].Name)

	svcByName := make(map[string]tracestore.ServiceSummary)
	for _, svc := range s.Services {
		svcByName[svc.Name] = svc
	}
	require.Contains(t, svcByName, "frontend")
	require.Contains(t, svcByName, "backend")
	assert.Equal(t, 2, svcByName["frontend"].SpanCount)
	assert.Equal(t, 1, svcByName["frontend"].ErrorSpanCount)
	assert.Equal(t, 1, svcByName["backend"].SpanCount)
	assert.Equal(t, 0, svcByName["backend"].ErrorSpanCount)
}

// TestComputeSummaries_MultiChunk verifies that a single trace split across
// multiple consecutive ptrace.Traces chunks produces exactly one summary.
func TestComputeSummaries_MultiChunk(t *testing.T) {
	// chunk 1: root span
	chunk1 := ptrace.NewTraces()
	rs1 := chunk1.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "svc")
	span1 := rs1.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span1.SetTraceID(testTraceID)
	span1.SetSpanID(pcommon.SpanID([8]byte{1}))
	span1.SetName("root")

	// chunk 2: child span, same trace ID
	chunk2 := ptrace.NewTraces()
	rs2 := chunk2.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "svc")
	span2 := rs2.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span2.SetTraceID(testTraceID)
	span2.SetSpanID(pcommon.SpanID([8]byte{2}))
	span2.SetParentSpanID(pcommon.SpanID([8]byte{1}))

	seq := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{chunk1, chunk2}, nil)
	})
	summaries, err := jiter.FlattenWithErrors(computeSummaries(seq, noopAdjuster))
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, 2, summaries[0].SpanCount)
	assert.Equal(t, "root", summaries[0].RootOperationName)
}

func TestSummarizeTrace_OrphanSpans(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "svc")
	scope := rs.ScopeSpans().AppendEmpty()

	root := scope.Spans().AppendEmpty()
	root.SetSpanID(pcommon.SpanID([8]byte{1}))
	// no parent → root

	// span with a parent not present in this trace → orphan
	orphan := scope.Spans().AppendEmpty()
	orphan.SetSpanID(pcommon.SpanID([8]byte{2}))
	orphan.SetParentSpanID(pcommon.SpanID([8]byte{0xFF}))

	summary := summarizeTrace(td)
	assert.Equal(t, 2, summary.SpanCount)
	assert.Equal(t, 1, summary.OrphanSpanCount)
}
