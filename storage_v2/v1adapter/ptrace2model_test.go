// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/crossdock/crossdock-go/require"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestPTracesSeq2ToModel_SuccessWithSinglePTraceInSeq(t *testing.T) {
	var (
		ProcessNoServiceName string    = "OTLPResourceNoServiceName"
		StartTime            time.Time = time.Unix(0, 0) // 1970-01-01T00:00:00Z, matches the default for otel span's start time
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
							Process:       model.NewProcess(ProcessNoServiceName, nil),
							StartTime:     StartTime,
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
				span1.SetTraceID(modelTraceID.ToOTELTraceID())
				span1.SetName("op-success-a")
				span1.SetSpanID(model.NewSpanID(1).ToOTELSpanID())

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
							Process:       model.NewProcess(ProcessNoServiceName, nil),
							StartTime:     StartTime,
						}, {
							TraceID:       model.NewTraceID(2, 3),
							SpanID:        model.NewSpanID(2),
							OperationName: "op-two-chunks-b",
							Process:       model.NewProcess(ProcessNoServiceName, nil),
							StartTime:     StartTime,
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
				span1.SetTraceID(modelTraceID.ToOTELTraceID())
				span1.SetName("op-two-chunks-a")
				span1.SetSpanID(model.NewSpanID(1).ToOTELSpanID())

				traceChunk2 := ptrace.NewTraces()
				rSpans2 := traceChunk2.ResourceSpans().AppendEmpty()
				sSpans2 := rSpans2.ScopeSpans().AppendEmpty()
				spans2 := sSpans2.Spans()
				span2 := spans2.AppendEmpty()
				span2.SetTraceID(modelTraceID.ToOTELTraceID())
				span2.SetName("op-two-chunks-b")
				span2.SetSpanID(model.NewSpanID(2).ToOTELSpanID())
				// Yield the test trace
				yield([]ptrace.Traces{traceChunk1, traceChunk2}, nil)
			},
			expectedErr: nil,
		},
		{
			// a case that occurs when no trace is contained in the iterator
			name:                "empty sequence",
			expectedModelTraces: nil,
			seqTrace: func(yield func([]ptrace.Traces, error) bool) {
			},
			expectedErr: spanstore.ErrTraceNotFound,
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
				span1.SetTraceID(modelTraceID.ToOTELTraceID())
				span1.SetName("op-error-a")
				span1.SetSpanID(model.NewSpanID(1).ToOTELSpanID())

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
			actualTraces, err := PTracesSeq2ToModel(tc.seqTrace)
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
					require.Equal(t, espan.TraceID, aSpans[j].TraceID)
					require.Equal(t, espan.OperationName, aSpans[j].OperationName)
					require.Equal(t, espan.Process, aSpans[j].Process)
				}
			}
		})
	}
}
