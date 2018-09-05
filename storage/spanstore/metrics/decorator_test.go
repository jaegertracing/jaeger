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
	mockReader.On("GetServices").Return([]string{}, nil)
	mrs.GetServices(context.Background())
	mockReader.On("GetOperations", "something").Return([]string{}, nil)
	mrs.GetOperations(context.Background(), "something")
	mockReader.On("GetTrace", model.TraceID{}).Return(&model.Trace{}, nil)
	mrs.GetTrace(context.Background(), model.TraceID{})
	mockReader.On("FindTraces", &spanstore.TraceQueryParameters{}).Return([]*model.Trace{}, nil)
	mrs.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
	counters, gauges := mf.Snapshot()
	expecteds := map[string]int64{
		"get_operations.attempts":  1,
		"get_operations.successes": 1,
		"get_operations.errors":    0,
		"get_trace.attempts":       1,
		"get_trace.successes":      1,
		"get_trace.errors":         0,
		"find_traces.attempts":     1,
		"find_traces.successes":    1,
		"find_traces.errors":       0,
		"get_services.attempts":    1,
		"get_services.successes":   1,
		"get_services.errors":      0,
	}

	existingKeys := []string{
		"get_operations.okLatency.P50",
		"get_trace.responses.P50",
		"find_traces.okLatency.P50", // this is not exhaustive
	}
	nonExistentKeys := []string{
		"get_operations.errLatency.P50",
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
	mockReader.On("GetServices").Return(nil, errors.New("Failure"))
	mrs.GetServices(context.Background())
	mockReader.On("GetOperations", "something").Return(nil, errors.New("Failure"))
	mrs.GetOperations(context.Background(), "something")
	mockReader.On("GetTrace", model.TraceID{}).Return(nil, errors.New("Failure"))
	mrs.GetTrace(context.Background(), model.TraceID{})
	mockReader.On("FindTraces", &spanstore.TraceQueryParameters{}).Return(nil, errors.New("Failure"))
	mrs.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
	counters, gauges := mf.Snapshot()
	expecteds := map[string]int64{
		"get_operations.attempts":  1,
		"get_operations.successes": 0,
		"get_operations.errors":    1,
		"get_trace.attempts":       1,
		"get_trace.successes":      0,
		"get_trace.errors":         1,
		"find_traces.attempts":     1,
		"find_traces.successes":    0,
		"find_traces.errors":       1,
		"get_services.attempts":    1,
		"get_services.successes":   0,
		"get_services.errors":      1,
	}

	existingKeys := []string{
		"get_operations.errLatency.P50",
	}

	nonExistentKeys := []string{
		"get_operations.okLatency.P50",
		"get_trace.responses.P50",
		"query.okLatency.P50", // this is not exhaustive
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, expecteds, gauges, existingKeys, nonExistentKeys)
}
