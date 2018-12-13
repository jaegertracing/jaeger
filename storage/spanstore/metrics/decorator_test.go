// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	. "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
	"github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestSuccessfulUnderlyingCalls(t *testing.T) {
	mf := metrics.NewLocalFactory(0)

	mockReader := mocks.Reader{}
	mrs := NewReadMetricsDecorator(&mockReader, mf)
	mockReader.On("GetServices", context.Background()).Return([]string{}, nil)
	mrs.GetServices(context.Background())
	mockReader.On("GetOperations", context.Background(), "something").Return([]string{}, nil)
	mrs.GetOperations(context.Background(), "something")
	mockReader.On("GetTrace", context.Background(), model.TraceID{}).Return(&model.Trace{}, nil)
	mrs.GetTrace(context.Background(), model.TraceID{})
	mockReader.On("FindTraces", context.Background(), &spanstore.TraceQueryParameters{}).Return([]*model.Trace{}, nil)
	mrs.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
	mockReader.On("FindTraceIDs", context.Background(), &spanstore.TraceQueryParameters{}).Return([]model.TraceID{}, nil)
	mrs.FindTraceIDs(context.Background(), &spanstore.TraceQueryParameters{})
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

func checkExpectedExistingAndNonExistentCounters(t *testing.T, actualCounters, expectedCounters, actualGauges map[string]int64, existingKeys, nonExistentKeys []string) {
	for k, v := range expectedCounters {
		assert.EqualValues(t, v, actualCounters[k], k)
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
	mf := metrics.NewLocalFactory(0)

	mockReader := mocks.Reader{}
	mrs := NewReadMetricsDecorator(&mockReader, mf)
	mockReader.On("GetServices", context.Background()).Return(nil, errors.New("Failure"))
	mrs.GetServices(context.Background())
	mockReader.On("GetOperations", context.Background(), "something").Return(nil, errors.New("Failure"))
	mrs.GetOperations(context.Background(), "something")
	mockReader.On("GetTrace", context.Background(), model.TraceID{}).Return(nil, errors.New("Failure"))
	mrs.GetTrace(context.Background(), model.TraceID{})
	mockReader.On("FindTraces", context.Background(), &spanstore.TraceQueryParameters{}).Return(nil, errors.New("Failure"))
	mrs.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
	mockReader.On("FindTraceIDs", context.Background(), &spanstore.TraceQueryParameters{}).Return(nil, errors.New("Failure"))
	mrs.FindTraceIDs(context.Background(), &spanstore.TraceQueryParameters{})
	counters, gauges := mf.Snapshot()
	expecteds := map[string]int64{
		"requests|operation=get_operations|result=ok":  0,
		"requests|operation=get_operations|result=err": 1,
		"requests|operation=get_trace|result=ok":       0,
		"requests|operation=get_trace|result=err":      1,
		"requests|operation=find_traces|result=ok":     0,
		"requests|operation=find_traces|result=err":    1,
		"requests|operation=find_trace_ids|result=ok":   0,
		"requests|operation=find_trace_ids|result=err":  1,
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
