// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestNewTraceReader(t *testing.T) {
	reader := NewTraceReader(&mocks.CoreSpanReader{})
	assert.NotNil(t, reader)
}

func TestGetServices(t *testing.T) {
	services := []string{"service-1", "service-2"}
	reader := mocks.CoreSpanReader{}
	reader.On("GetServices", mock.Anything).Return(services, nil)
	tracereader := &TraceReader{reader: &reader}
	got, err := tracereader.GetServices(context.Background())
	require.NoError(t, err)
	require.Equal(t, services, got)
}

func TestGetOperationsErr(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	reader.On("GetOperations", mock.Anything, mock.Anything).Return(nil, errors.New("error"))
	tracereader := &TraceReader{reader: &reader}
	got, err := tracereader.GetOperations(context.Background(), tracestore.OperationQueryParams{
		ServiceName: "service-1",
		SpanKind:    "some kind",
	})
	require.ErrorContains(t, err, "error")
	require.Nil(t, got)
}

func TestGetOperations(t *testing.T) {
	reader := mocks.CoreSpanReader{}
	ops := []dbmodel.Operation{
		{
			ServiceName:   "service-1",
			SpanKind:      "some kind",
			OperationName: "operation-1",
		},
		{
			ServiceName:   "service-1",
			SpanKind:      "some kind",
			OperationName: "operation-2",
		},
	}
	reader.On("GetOperations", mock.Anything, mock.Anything).Return(ops, nil)
	tracereader := &TraceReader{reader: &reader}
	got, err := tracereader.GetOperations(context.Background(), tracestore.OperationQueryParams{
		ServiceName: "service-1",
		SpanKind:    "some kind",
	})
	require.NoError(t, err)
	expected := []tracestore.Operation{
		{
			Name:     "operation-1",
			SpanKind: "some kind",
		},
		{
			Name:     "operation-2",
			SpanKind: "some kind",
		},
	}
	require.Equal(t, expected, got)
}

// Tests from here are copied directly from v1 adapter.

func TestTraceReader_GetTracesErrorResponse(t *testing.T) {
	testCases := []struct {
		name          string
		firstErr      error
		expectedErr   error
		expectedIters int
	}{
		{
			name:          "real error aborts iterator",
			firstErr:      assert.AnError,
			expectedErr:   assert.AnError,
			expectedIters: 0, // technically 1 but FlattenWithErrors makes it 0
		},
		{
			name:          "trace not found error skips iteration",
			firstErr:      spanstore.ErrTraceNotFound,
			expectedErr:   nil,
			expectedIters: 1,
		},
		{
			name:          "no error produces two iterations",
			firstErr:      nil,
			expectedErr:   nil,
			expectedIters: 2,
		},
	}
	traceID := func(i byte) tracestore.GetTraceParams {
		return tracestore.GetTraceParams{
			TraceID: pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, i}),
		}
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			sr := &mocks.CoreSpanReader{}
			sr.On("GetTrace", mock.Anything, mock.Anything).Return(&model.Trace{}, test.firstErr).Once()
			sr.On("GetTrace", mock.Anything, mock.Anything).Return(&model.Trace{}, nil).Once()
			traceReader := &TraceReader{
				reader: sr,
			}
			traces, err := jiter.FlattenWithErrors(traceReader.GetTraces(
				context.Background(), traceID(1), traceID(2),
			))
			require.ErrorIs(t, err, test.expectedErr)
			assert.Len(t, traces, test.expectedIters)
		})
	}
}

func TestTraceReader_GetTracesEarlyStop(t *testing.T) {
	sr := &mocks.CoreSpanReader{}
	sr.On(
		"GetTrace",
		mock.Anything,
		mock.Anything,
	).Return(&model.Trace{}, nil)
	traceReader := &TraceReader{
		reader: sr,
	}
	traceID := func(i byte) tracestore.GetTraceParams {
		return tracestore.GetTraceParams{
			TraceID: pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, i}),
		}
	}
	called := 0
	traceReader.GetTraces(
		context.Background(), traceID(1), traceID(2), traceID(3),
	)(func(tr []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		require.Len(t, tr, 1)
		called++
		return true
	})
	assert.Equal(t, 3, called)
	called = 0
	traceReader.GetTraces(
		context.Background(), traceID(1), traceID(2), traceID(3),
	)(func(tr []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		require.Len(t, tr, 1)
		called++
		return false // early return
	})
	assert.Equal(t, 1, called)
}

func TestTraceReader_FindTracesDelegatesSuccessResponse(t *testing.T) {
	modelTraces := []*model.Trace{
		{
			Spans: []*model.Span{
				{
					TraceID:       model.NewTraceID(2, 3),
					SpanID:        model.SpanID(1),
					OperationName: "operation-a",
				},
				{
					TraceID:       model.NewTraceID(4, 5),
					SpanID:        model.SpanID(2),
					OperationName: "operation-b",
				},
			},
		},
		{
			Spans: []*model.Span{
				{
					TraceID:       model.NewTraceID(6, 7),
					SpanID:        model.SpanID(3),
					OperationName: "operation-c",
				},
			},
		},
	}
	sr := &mocks.CoreSpanReader{}
	now := time.Now()
	sr.On(
		"FindTraces",
		mock.Anything,
		&spanstore.TraceQueryParameters{
			ServiceName:   "service",
			OperationName: "operation",
			Tags:          map[string]string{"tag-a": "val-a"},
			StartTimeMin:  now,
			StartTimeMax:  now.Add(time.Minute),
			DurationMin:   time.Minute,
			DurationMax:   time.Hour,
			NumTraces:     10,
		},
	).Return(modelTraces, nil)
	traceReader := &TraceReader{
		reader: sr,
	}
	attributes := pcommon.NewMap()
	attributes.PutStr("tag-a", "val-a")
	traces, err := jiter.FlattenWithErrors(traceReader.FindTraces(
		context.Background(),
		tracestore.TraceQueryParams{
			ServiceName:   "service",
			OperationName: "operation",
			Attributes:    attributes,
			StartTimeMin:  now,
			StartTimeMax:  now.Add(time.Minute),
			DurationMin:   time.Minute,
			DurationMax:   time.Hour,
			SearchDepth:   10,
		},
	))
	require.NoError(t, err)
	require.Len(t, traces, len(modelTraces))
	traceASpans := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	traceBSpans := traces[1].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3}, traceASpans.At(0).TraceID())
	require.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 1}, traceASpans.At(0).SpanID())
	require.Equal(t, "operation-a", traceASpans.At(0).Name())
	require.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 5}, traceASpans.At(1).TraceID())
	require.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 2}, traceASpans.At(1).SpanID())
	require.Equal(t, "operation-b", traceASpans.At(1).Name())
	require.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0, 7}, traceBSpans.At(0).TraceID())
	require.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 3}, traceBSpans.At(0).SpanID())
	require.Equal(t, "operation-c", traceBSpans.At(0).Name())
}

func TestTraceReader_FindTracesEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		modelTraces    []*model.Trace
		expectedTraces []ptrace.Traces
		err            error
	}{
		{
			name:           "nil response",
			modelTraces:    nil,
			expectedTraces: nil,
		},
		{
			name:           "empty response",
			modelTraces:    []*model.Trace{},
			expectedTraces: nil,
		},
		{
			name:           "error response",
			modelTraces:    nil,
			expectedTraces: nil,
			err:            errors.New("test error"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sr := &mocks.CoreSpanReader{}
			sr.On(
				"FindTraces",
				mock.Anything,
				mock.Anything,
			).Return(test.modelTraces, test.err)
			traceReader := &TraceReader{
				reader: sr,
			}
			traces, err := jiter.FlattenWithErrors(traceReader.FindTraces(
				context.Background(),
				tracestore.TraceQueryParams{
					Attributes: pcommon.NewMap(),
				},
			))
			require.ErrorIs(t, err, test.err)
			require.Equal(t, test.expectedTraces, traces)
		})
	}
}

func TestTraceReader_FindTracesEarlyStop(t *testing.T) {
	sr := &mocks.CoreSpanReader{}
	sr.On(
		"FindTraces",
		mock.Anything,
		mock.Anything,
	).Return([]*model.Trace{{}, {}, {}}, nil).Twice()
	traceReader := &TraceReader{
		reader: sr,
	}
	called := 0
	traceReader.FindTraces(
		context.Background(), tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(),
		},
	)(func(tr []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		require.Len(t, tr, 1)
		called++
		return true
	})
	assert.Equal(t, 3, called)
	called = 0
	traceReader.FindTraces(
		context.Background(), tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(),
		},
	)(func(tr []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		require.Len(t, tr, 1)
		called++
		return false // early return
	})
	assert.Equal(t, 1, called)
}

func TestTraceReader_FindTraceIDsDelegatesResponse(t *testing.T) {
	tests := []struct {
		name             string
		modelTraceIDs    []model.TraceID
		expectedTraceIDs []tracestore.FoundTraceID
		err              error
	}{
		{
			name: "successful response",
			modelTraceIDs: []model.TraceID{
				{Low: 3, High: 2},
				{Low: 4, High: 3},
			},
			expectedTraceIDs: []tracestore.FoundTraceID{
				{
					TraceID: pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3}),
				},
				{
					TraceID: pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 4}),
				},
			},
		},
		{
			name:             "empty response",
			modelTraceIDs:    []model.TraceID{},
			expectedTraceIDs: nil,
		},
		{
			name:             "nil response",
			modelTraceIDs:    nil,
			expectedTraceIDs: nil,
		},
		{
			name:             "error response",
			modelTraceIDs:    nil,
			expectedTraceIDs: nil,
			err:              errors.New("test error"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sr := &mocks.CoreSpanReader{}
			now := time.Now()
			sr.On(
				"FindTraceIDs",
				mock.Anything,
				&spanstore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					Tags:          map[string]string{"tag-a": "val-a"},
					StartTimeMin:  now,
					StartTimeMax:  now.Add(time.Minute),
					DurationMin:   time.Minute,
					DurationMax:   time.Hour,
					NumTraces:     10,
				},
			).Return(test.modelTraceIDs, test.err)
			traceReader := &TraceReader{
				reader: sr,
			}
			attributes := pcommon.NewMap()
			attributes.PutStr("tag-a", "val-a")
			traceIDs, err := jiter.FlattenWithErrors(traceReader.FindTraceIDs(
				context.Background(),
				tracestore.TraceQueryParams{
					ServiceName:   "service",
					OperationName: "operation",
					Attributes:    attributes,
					StartTimeMin:  now,
					StartTimeMax:  now.Add(time.Minute),
					DurationMin:   time.Minute,
					DurationMax:   time.Hour,
					SearchDepth:   10,
				},
			))
			require.ErrorIs(t, err, test.err)
			require.Equal(t, test.expectedTraceIDs, traceIDs)
		})
	}
}
