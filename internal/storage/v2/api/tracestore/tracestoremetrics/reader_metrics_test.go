// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestoremetrics

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	mockReader.On("GetServices", context.Background()).Return([]string{"service-x"}, nil)
	mrs.GetServices(context.Background())
	operationQuery := tracestore.OperationQueryParams{ServiceName: "something"}
	mockReader.On("GetOperations", context.Background(), operationQuery).
		Return([]tracestore.Operation{{}}, nil)
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
	expected := map[string]int64{
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
		"responses|operation=get_trace":                2,
		"responses|operation=find_traces":              2,
		"responses|operation=find_trace_ids":           2,
		"responses|operation=get_operations":           1,
		"responses|operation=get_services":             1,
	}

	existingKeys := []string{
		"latency|operation=get_operations|result=ok.P50",
		"latency|operation=find_traces|result=ok.P50", // this is not exhaustive
	}
	nonExistentKeys := []string{
		"latency|operation=get_operations|result=err.P50",
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, expected, gauges, existingKeys, nonExistentKeys)
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
		assert.True(t, ok, k)
	}

	for _, k := range nonExistentKeys {
		_, ok := actualGauges[k]
		assert.False(t, ok, k)
	}
}

func TestFailingUnderlyingCalls(t *testing.T) {
	mf := metricstest.NewFactory(0)

	mockReader := mocks.Reader{}
	mrs := NewReaderDecorator(&mockReader, mf)
	returningErr := assert.AnError
	mockReader.On("GetServices", context.Background()).
		Return(nil, returningErr)
	mrs.GetServices(context.Background())
	operationQuery := tracestore.OperationQueryParams{ServiceName: "something"}
	mockReader.On("GetOperations", context.Background(), operationQuery).
		Return(nil, returningErr)
	mrs.GetOperations(context.Background(), operationQuery)
	mockReader.On("GetTraces", context.Background(), []tracestore.GetTraceParams{{}}).
		Return(emptyIter[ptrace.Traces](nil, returningErr))
	for range mrs.GetTraces(context.Background(), tracestore.GetTraceParams{}) {
		t.Log("GetTraces iteration")
	}
	mockReader.On("FindTraces", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[ptrace.Traces](nil, returningErr))
	for range mrs.FindTraces(context.Background(), tracestore.TraceQueryParams{}) {
		t.Log("FindTraces iteration")
	}
	mockReader.On("FindTraceIDs", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[tracestore.FoundTraceID](nil, returningErr))
	for range mrs.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{}) {
		t.Log("FindTraceIDs iteration")
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
		"latency|operation=query|result=ok.P50", // this is not exhaustive
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, expecteds, gauges, existingKeys, nonExistentKeys)
}

func emptyIter[T any](td []T, err error) iter.Seq2[[]T, error] {
	return func(yield func([]T, error) bool) {
		if err != nil {
			yield(nil, err)
			return
		}
		for _, t := range td {
			if !yield([]T{t}, nil) {
				return
			}
		}
	}
}

func TestReadMetricsDecorator_FindTraceSummaries(t *testing.T) {
	mf := metricstest.NewFactory(0)

	inner := &mocks.Reader{}
	summaries := []tracestore.TraceSummary{{RootServiceName: "svc-a"}, {RootServiceName: "svc-b"}}
	inner.On("FindTraceSummaries", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[tracestore.TraceSummary](summaries, nil))

	d := NewReaderDecorator(inner, mf)

	var got []tracestore.TraceSummary
	for batch, err := range d.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{}) {
		require.NoError(t, err)
		got = append(got, batch...)
	}
	assert.Len(t, got, len(summaries))

	counters, _ := mf.Snapshot()
	assert.Equal(t, int64(1), counters["requests|operation=find_trace_summaries|result=ok"])
	assert.Equal(t, int64(int64(len(summaries))), counters["responses|operation=find_trace_summaries"])
}

func TestReadMetricsDecorator_FindTraceSummaries_Error(t *testing.T) {
	mf := metricstest.NewFactory(0)

	inner := &mocks.Reader{}
	inner.On("FindTraceSummaries", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[tracestore.TraceSummary](nil, assert.AnError))

	d := NewReaderDecorator(inner, mf)
	for range d.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{}) {
		t.Log("FindTraceSummaries error iteration")
	}

	counters, _ := mf.Snapshot()
	assert.Equal(t, int64(1), counters["requests|operation=find_trace_summaries|result=err"])
}

func TestReadMetricsDecorator_FindTraceSummaries_EarlyExit(t *testing.T) {
	mf := metricstest.NewFactory(0)

	inner := &mocks.Reader{}
	// emptyIter yields each summary as its own batch. The consumer stops after the
	// second batch, exercising the !yield early-exit path inside FindTraceSummaries:
	// the third summary must never be delivered.
	summaries := []tracestore.TraceSummary{
		{RootServiceName: "svc-a"},
		{RootServiceName: "svc-b"},
		{RootServiceName: "svc-c"},
	}
	inner.On("FindTraceSummaries", context.Background(), tracestore.TraceQueryParams{}).
		Return(emptyIter[tracestore.TraceSummary](summaries, nil))

	d := NewReaderDecorator(inner, mf)
	var got []tracestore.TraceSummary
	for batch, err := range d.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{}) {
		require.NoError(t, err)
		got = append(got, batch...)
		if len(got) == 2 {
			break
		}
	}

	// The consumer received exactly the first two summaries; the third was never yielded.
	require.Len(t, got, 2)
	assert.Equal(t, "svc-a", got[0].RootServiceName)
	assert.Equal(t, "svc-b", got[1].RootServiceName)

	// The deferred metrics emit still runs on early exit, counting the batches
	// delivered before the break as a successful operation.
	counters, _ := mf.Snapshot()
	assert.Equal(t, int64(1), counters["requests|operation=find_trace_summaries|result=ok"])
	assert.Equal(t, int64(2), counters["responses|operation=find_trace_summaries"])
}
