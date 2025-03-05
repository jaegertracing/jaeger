// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestProtoFromTraces_AddsWarnings(t *testing.T) {
	traces := ptrace.NewTraces()
	rs1 := traces.ResourceSpans().AppendEmpty()
	ss1 := rs1.ScopeSpans().AppendEmpty()
	span1 := ss1.Spans().AppendEmpty()
	span1.SetName("test-span-1")
	span1.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	jptrace.AddWarnings(span1, "test-warning-1")
	jptrace.AddWarnings(span1, "test-warning-2")
	span1.Attributes().PutStr("key", "value")

	ss2 := rs1.ScopeSpans().AppendEmpty()
	span2 := ss2.Spans().AppendEmpty()
	span2.SetName("test-span-2")
	span2.SetSpanID(pcommon.SpanID([8]byte{9, 10, 11, 12, 13, 14, 15, 16}))

	rs2 := traces.ResourceSpans().AppendEmpty()
	ss3 := rs2.ScopeSpans().AppendEmpty()
	span3 := ss3.Spans().AppendEmpty()
	span3.SetName("test-span-3")
	span3.SetSpanID(pcommon.SpanID([8]byte{17, 18, 19, 20, 21, 22, 23, 24}))
	jptrace.AddWarnings(span3, "test-warning-3")

	batches := ProtoFromTraces(traces)

	assert.Len(t, batches, 2)

	assert.Len(t, batches[0].Spans, 2)
	assert.Equal(t, "test-span-1", batches[0].Spans[0].OperationName)
	assert.Equal(t, []string{"test-warning-1", "test-warning-2"}, batches[0].Spans[0].Warnings)
	assert.Equal(t, []model.KeyValue{{Key: "key", VStr: "value"}}, batches[0].Spans[0].Tags)
	assert.Equal(t, "test-span-2", batches[0].Spans[1].OperationName)
	assert.Empty(t, batches[0].Spans[1].Warnings)
	assert.Empty(t, batches[0].Spans[1].Tags)

	assert.Len(t, batches[1].Spans, 1)
	assert.Equal(t, "test-span-3", batches[1].Spans[0].OperationName)
	assert.Equal(t, []string{"test-warning-3"}, batches[1].Spans[0].Warnings)
	assert.Empty(t, batches[1].Spans[0].Tags)
}

func TestProtoToTraces_AddsWarnings(t *testing.T) {
	batch1 := &model.Batch{
		Process: &model.Process{
			ServiceName: "batch-1",
		},
		Spans: []*model.Span{
			{
				OperationName: "test-span-1",
				SpanID:        model.NewSpanID(1),
				Warnings:      []string{"test-warning-1", "test-warning-2"},
			},
			{
				OperationName: "test-span-2",
				SpanID:        model.NewSpanID(2),
			},
		},
	}
	batch2 := &model.Batch{
		Process: &model.Process{
			ServiceName: "batch-2",
		},
		Spans: []*model.Span{
			{
				OperationName: "test-span-3",
				SpanID:        model.NewSpanID(3),
				Warnings:      []string{"test-warning-3"},
			},
		},
	}
	batches := []*model.Batch{batch1, batch2}
	traces := V1BatchesToTraces(batches)

	assert.Equal(t, 2, traces.ResourceSpans().Len())

	spanMap := jptrace.SpanMap(traces, func(s ptrace.Span) string {
		return s.Name()
	})

	span1 := spanMap["test-span-1"]
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}), span1.SpanID())
	assert.Equal(t, []string{"test-warning-1", "test-warning-2"}, jptrace.GetWarnings(span1))

	span2 := spanMap["test-span-2"]
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}), span2.SpanID())
	assert.Empty(t, jptrace.GetWarnings(span2))

	span3 := spanMap["test-span-3"]
	assert.Equal(t, pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}), span3.SpanID())
	assert.Equal(t, []string{"test-warning-3"}, jptrace.GetWarnings(span3))
}

func TestV1TracesFromSeq2(t *testing.T) {
	var (
		processNoServiceName string    = "OTLPResourceNoServiceName"
		startTime            time.Time = time.Unix(0, 0) // 1970-01-01T00:00:00Z, matches the default for otel span's start time
	)

	testCases := []struct {
		name                string
		expectedModelTraces []*model.Trace
		seqTrace            iter.Seq2[[]ptrace.Traces, error]
		expectedErr         error
	}{
		{
			name: "sequence with one trace",
			expectedModelTraces: []*model.Trace{
				{
					Spans: []*model.Span{
						{
							TraceID:       model.NewTraceID(2, 3),
							SpanID:        model.NewSpanID(1),
							OperationName: "op-success-a",
							Process:       model.NewProcess(processNoServiceName, make([]model.KeyValue, 0)),
							StartTime:     startTime,
						},
					},
				},
			},
			seqTrace: func(yield func([]ptrace.Traces, error) bool) {
				testTrace := ptrace.NewTraces()
				rSpans := testTrace.ResourceSpans().AppendEmpty()
				sSpans := rSpans.ScopeSpans().AppendEmpty()
				spans := sSpans.Spans()

				// Add a new span and set attributes
				modelTraceID := model.NewTraceID(2, 3)
				span1 := spans.AppendEmpty()
				span1.SetTraceID(FromV1TraceID(modelTraceID))
				span1.SetName("op-success-a")
				span1.SetSpanID(FromV1SpanID(model.NewSpanID(1)))

				// Yield the test trace
				yield([]ptrace.Traces{testTrace}, nil)
			},
			expectedErr: nil,
		},
		{
			name: "sequence with two chunks of a trace",
			expectedModelTraces: []*model.Trace{
				{
					Spans: []*model.Span{
						{
							TraceID:       model.NewTraceID(2, 3),
							SpanID:        model.NewSpanID(1),
							OperationName: "op-two-chunks-a",
							Process:       model.NewProcess(processNoServiceName, make([]model.KeyValue, 0)),
							StartTime:     startTime,
						}, {
							TraceID:       model.NewTraceID(2, 3),
							SpanID:        model.NewSpanID(2),
							OperationName: "op-two-chunks-b",
							Process:       model.NewProcess(processNoServiceName, make([]model.KeyValue, 0)),
							StartTime:     startTime,
						},
					},
				},
			},
			seqTrace: func(yield func([]ptrace.Traces, error) bool) {
				traceChunk1 := ptrace.NewTraces()
				rSpans1 := traceChunk1.ResourceSpans().AppendEmpty()
				sSpans1 := rSpans1.ScopeSpans().AppendEmpty()
				spans1 := sSpans1.Spans()
				modelTraceID := model.NewTraceID(2, 3)
				span1 := spans1.AppendEmpty()
				span1.SetTraceID(FromV1TraceID(modelTraceID))
				span1.SetName("op-two-chunks-a")
				span1.SetSpanID(FromV1SpanID(model.NewSpanID(1)))

				traceChunk2 := ptrace.NewTraces()
				rSpans2 := traceChunk2.ResourceSpans().AppendEmpty()
				sSpans2 := rSpans2.ScopeSpans().AppendEmpty()
				spans2 := sSpans2.Spans()
				span2 := spans2.AppendEmpty()
				span2.SetTraceID(FromV1TraceID(modelTraceID))
				span2.SetName("op-two-chunks-b")
				span2.SetSpanID(FromV1SpanID(model.NewSpanID(2)))
				// Yield the test trace
				yield([]ptrace.Traces{traceChunk1, traceChunk2}, nil)
			},
			expectedErr: nil,
		},
		{
			// a case that occurs when no trace is contained in the iterator
			name:                "empty sequence",
			expectedModelTraces: nil,
			seqTrace:            func(_ func([]ptrace.Traces, error) bool) {},
			expectedErr:         nil,
		},
		{
			name:                "sequence containing error",
			expectedModelTraces: nil,
			seqTrace: func(yield func([]ptrace.Traces, error) bool) {
				testTrace := ptrace.NewTraces()
				rSpans := testTrace.ResourceSpans().AppendEmpty()
				sSpans := rSpans.ScopeSpans().AppendEmpty()
				spans := sSpans.Spans()

				modelTraceID := model.NewTraceID(2, 3)
				span1 := spans.AppendEmpty()
				span1.SetTraceID(FromV1TraceID(modelTraceID))
				span1.SetName("op-error-a")
				span1.SetSpanID(FromV1SpanID(model.NewSpanID(1)))

				// Yield the test trace
				if !yield([]ptrace.Traces{testTrace}, nil) {
					return
				}
				yield(nil, errors.New("unexpected-op-err"))
			},
			expectedErr: errors.New("unexpected-op-err"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualTraces, err := V1TracesFromSeq2(tc.seqTrace)
			require.Equal(t, tc.expectedErr, err)
			require.Equal(t, len(tc.expectedModelTraces), len(actualTraces))
			if len(tc.expectedModelTraces) < 1 {
				return
			}
			for i, etrace := range tc.expectedModelTraces {
				eSpans := etrace.Spans
				aSpans := actualTraces[i].Spans
				require.Equal(t, len(eSpans), len(aSpans))
				for j, espan := range eSpans {
					assert.Equal(t, espan.TraceID, aSpans[j].TraceID)
					assert.Equal(t, espan.OperationName, aSpans[j].OperationName)
					assert.Equal(t, espan.Process, aSpans[j].Process)
				}
			}
		})
	}
}

func TestV1TraceToOtelTrace_ReturnsExptectedOtelTrace(t *testing.T) {
	jTrace := &model.Trace{
		Spans: []*model.Span{
			{
				TraceID:       model.NewTraceID(2, 3),
				SpanID:        model.NewSpanID(1),
				Process:       model.NewProcess("Service1", nil),
				OperationName: "two-resources-1",
			}, {
				TraceID:       model.NewTraceID(2, 3),
				SpanID:        model.NewSpanID(2),
				Process:       model.NewProcess("service2", nil),
				OperationName: "two-resources-2",
			},
		},
	}
	actualTrace := V1TraceToOtelTrace(jTrace)

	require.NotEmpty(t, actualTrace)
	require.Equal(t, 2, actualTrace.ResourceSpans().Len())
}

func TestV1TraceToOtelTrace_ReturnEmptyOtelTrace(t *testing.T) {
	jTrace := &model.Trace{}
	eTrace := ptrace.NewTraces()
	aTrace := V1TraceToOtelTrace(jTrace)

	require.Equal(t, eTrace.SpanCount(), aTrace.SpanCount(), 0)
}

func TestV1TraceIDsFromSeq2(t *testing.T) {
	testCases := []struct {
		name          string
		seqTraceIDs   iter.Seq2[[]tracestore.FoundTraceID, error]
		expectedIDs   []model.TraceID
		expectedError error
	}{
		{
			name:          "empty sequence",
			seqTraceIDs:   func(func([]tracestore.FoundTraceID, error) bool) {},
			expectedIDs:   nil,
			expectedError: nil,
		},
		{
			name: "sequence with error",
			seqTraceIDs: func(yield func([]tracestore.FoundTraceID, error) bool) {
				yield(nil, assert.AnError)
			},
			expectedIDs:   nil,
			expectedError: assert.AnError,
		},
		{
			name: "sequence with one chunk of trace IDs",
			seqTraceIDs: func(yield func([]tracestore.FoundTraceID, error) bool) {
				yield([]tracestore.FoundTraceID{
					{
						TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3}),
					},
					{
						TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 5}),
					},
				}, nil)
			},
			expectedIDs: []model.TraceID{
				model.NewTraceID(2, 3),
				model.NewTraceID(4, 5),
			},
			expectedError: nil,
		},
		{
			name: "sequence with multiple chunks of trace IDs",
			seqTraceIDs: func(yield func([]tracestore.FoundTraceID, error) bool) {
				traceID1 := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3})
				traceID2 := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 5})
				traceID3 := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 7})
				yield([]tracestore.FoundTraceID{{TraceID: traceID1}}, nil)
				yield([]tracestore.FoundTraceID{{TraceID: traceID2}, {TraceID: traceID3}}, nil)
			},
			expectedIDs: []model.TraceID{
				model.NewTraceID(2, 3),
				model.NewTraceID(4, 5),
				model.NewTraceID(6, 7),
			},
			expectedError: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualIDs, err := V1TraceIDsFromSeq2(tc.seqTraceIDs)
			require.Equal(t, tc.expectedError, err)
			require.Equal(t, tc.expectedIDs, actualIDs)
		})
	}
}
