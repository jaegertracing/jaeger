// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestoremetrics

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func TestSuccessfulUnderlyingCalls(t *testing.T) {
	mf := metricstest.NewFactory(0)

	mockReader := mocks.Reader{}
	mrs := NewReaderDecorator(&mockReader, mf)
	traces := []ptrace.Traces{ptrace.NewTraces(), ptrace.NewTraces()}
	mockReader.On("GetServices", context.Background()).Return([]string{}, nil)
	mrs.GetServices(context.Background())
	operationQuery := tracestore.OperationQueryParams{ServiceName: "something"}
	mockReader.On("GetOperations", context.Background(), operationQuery).
		Return([]tracestore.Operation{}, nil)
	mrs.GetOperations(context.Background(), operationQuery)
	mockReader.On("GetTraces", context.Background(), []tracestore.GetTraceParams{{}}).Return(emptyIter[ptrace.Traces](traces, nil))
	count := 0
	for range mrs.GetTraces(context.Background(), tracestore.GetTraceParams{}) {
		if count != 0 {
			break
		}
		count++
	}
	mockReader.On("FindTraces", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[ptrace.Traces](traces, nil))
	count = 0
	for range mrs.FindTraces(context.Background(), tracestore.TraceQueryParams{}) {
		if count != 0 {
			break
		}
		count++
	}
	mockReader.On("FindTraceIDs", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[tracestore.FoundTraceID]([]tracestore.FoundTraceID{{TraceID: [16]byte{}}, {TraceID: [16]byte{}}}, nil))
	count = 0
	for range mrs.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{}) {
		if count != 0 {
			break
		}
		count++
	}
	counters, gauges := mf.Snapshot()
	expecteds := map[string]int64{
		"requests|operation=get_operations|result=ok":  1,
		"requests|operation=get_operations|result=err": 0,
		"requests|operation=get_trace|result=ok":       1,
		"requests|operation=get_trace|result=err":      0,
		"requests|operation=find_traces|result=ok":     1,
		"requests|operation=find_traces|result=err":    0,
		"requests|operation=find_trace_ids|result=ok":  1,
		"requests|operation=find_trace_ids|result=err": 0,
		"requests|operation=get_services|result=ok":    1,
		"requests|operation=get_services|result=err":   0,
	}

	existingKeys := []string{
		"latency|operation=get_operations|result=ok.P50",
		"responses|operation=get_trace.P50",
		"latency|operation=find_traces|result=ok.P50", // this is not exhaustive
	}
	nonExistentKeys := []string{
		"latency|operation=get_operations|result=err.P50",
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, expecteds, gauges, existingKeys, nonExistentKeys)
}

func checkExpectedExistingAndNonExistentCounters(t *testing.T,
	actualCounters,
	expectedCounters,
	actualGauges map[string]int64,
	existingKeys,
	nonExistentKeys []string,
) {
	for k, v := range expectedCounters {
		assert.Equal(t, v, actualCounters[k], k)
	}

	for _, k := range existingKeys {
		_, ok := actualGauges[k]
		assert.True(t, ok)
	}

	for _, k := range nonExistentKeys {
		_, ok := actualGauges[k]
		assert.False(t, ok)
	}
}

func TestFailingUnderlyingCalls(t *testing.T) {
	mf := metricstest.NewFactory(0)

	mockReader := mocks.Reader{}
	mrs := NewReaderDecorator(&mockReader, mf)
	returningErr := errors.New("Failure")
	mockReader.On("GetServices", context.Background()).
		Return(nil, returningErr)
	mrs.GetServices(context.Background())
	operationQuery := tracestore.OperationQueryParams{ServiceName: "something"}
	mockReader.On("GetOperations", context.Background(), operationQuery).
		Return(nil, errors.New("Failure"))
	mrs.GetOperations(context.Background(), operationQuery)
	mockReader.On("GetTraces", context.Background(), []tracestore.GetTraceParams{{}}).
		Return(emptyIter[ptrace.Traces](nil, returningErr))
	//nolint:revive
	for range mrs.GetTraces(context.Background(), tracestore.GetTraceParams{}) {
		// It is necessary to range the iter to emit metrics, therefore this empty loop is present
	}
	mockReader.On("FindTraces", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[ptrace.Traces](nil, returningErr))
	//nolint:revive
	for range mrs.FindTraces(context.Background(), tracestore.TraceQueryParams{}) {
	}
	mockReader.On("FindTraceIDs", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[tracestore.FoundTraceID](nil, returningErr))
	//nolint:revive
	for range mrs.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{}) {
	}
	counters, gauges := mf.Snapshot()
	expecteds := map[string]int64{
		"requests|operation=get_operations|result=ok":  0,
		"requests|operation=get_operations|result=err": 1,
		"requests|operation=get_trace|result=ok":       0,
		"requests|operation=get_trace|result=err":      1,
		"requests|operation=find_traces|result=ok":     0,
		"requests|operation=find_traces|result=err":    1,
		"requests|operation=find_trace_ids|result=ok":  0,
		"requests|operation=find_trace_ids|result=err": 1,
		"requests|operation=get_services|result=ok":    0,
		"requests|operation=get_services|result=err":   1,
	}

	existingKeys := []string{
		"latency|operation=get_operations|result=err.P50",
	}

	nonExistentKeys := []string{
		"latency|operation=get_operations|result=ok.P50",
		"responses|operation=get_trace.P50",
		"latency|operation=query|result=ok.P50", // this is not exhaustive
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, expecteds, gauges, existingKeys, nonExistentKeys)
}

func emptyIter[T any](td []T, err error) iter.Seq2[[]T, error] {
	return func(yield func([]T, error) bool) {
		if err == nil {
			for _, t := range td {
				if !yield([]T{t}, nil) {
					return
				}
			}
		} else {
			yield(nil, err)
		}
	}
}
