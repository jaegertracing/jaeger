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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
	. "github.com/uber/jaeger/storage/spanstore/metrics"
	"github.com/uber/jaeger/storage/spanstore/mocks"
)

func TestSuccessfulUnderlyingCalls(t *testing.T) {
	mf := metrics.NewLocalFactory(0)

	mockReader := mocks.Reader{}
	mrs := NewReadMetricsDecorator(&mockReader, mf)
	mockReader.On("GetServices").Return([]string{}, nil)
	mrs.GetServices()
	mockReader.On("GetOperations", "something").Return([]string{}, nil)
	mrs.GetOperations("something")
	mockReader.On("GetTrace", model.TraceID{}).Return(&model.Trace{}, nil)
	mrs.GetTrace(model.TraceID{})
	mockReader.On("FindTraces", &spanstore.TraceQueryParameters{}).Return([]*model.Trace{}, nil)
	mrs.FindTraces(&spanstore.TraceQueryParameters{})
	counters, gauges := mf.Snapshot()
	expecteds := map[string]int64{
		"GetOperations.attempts":  1,
		"GetOperations.successes": 1,
		"GetOperations.errors":    0,
		"GetTrace.attempts":       1,
		"GetTrace.successes":      1,
		"GetTrace.errors":         0,
		"FindTraces.attempts":     1,
		"FindTraces.successes":    1,
		"FindTraces.errors":       0,
		"GetServices.attempts":    1,
		"GetServices.successes":   1,
		"GetServices.errors":      0,
	}

	existingKeys := []string{
		"GetOperations.okLatency.P50",
		"GetTrace.responses.P50",
		"FindTraces.okLatency.P50", // this is not exhaustive
	}
	nonExistentKeys := []string{
		"GetOperations.errLatency.P50",
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
	mrs.GetServices()
	mockReader.On("GetOperations", "something").Return(nil, errors.New("Failure"))
	mrs.GetOperations("something")
	mockReader.On("GetTrace", model.TraceID{}).Return(nil, errors.New("Failure"))
	mrs.GetTrace(model.TraceID{})
	mockReader.On("FindTraces", &spanstore.TraceQueryParameters{}).Return(nil, errors.New("Failure"))
	mrs.FindTraces(&spanstore.TraceQueryParameters{})
	counters, gauges := mf.Snapshot()
	expecteds := map[string]int64{
		"GetOperations.attempts":  1,
		"GetOperations.successes": 0,
		"GetOperations.errors":    1,
		"GetTrace.attempts":       1,
		"GetTrace.successes":      0,
		"GetTrace.errors":         1,
		"FindTraces.attempts":     1,
		"FindTraces.successes":    0,
		"FindTraces.errors":       1,
		"GetServices.attempts":    1,
		"GetServices.successes":   0,
		"GetServices.errors":      1,
	}

	existingKeys := []string{
		"GetOperations.errLatency.P50",
	}

	nonExistentKeys := []string{
		"GetOperations.okLatency.P50",
		"GetTrace.responses.P50",
		"Query.okLatency.P50", // this is not exhaustive
	}

	checkExpectedExistingAndNonExistentCounters(t, counters, expecteds, gauges, existingKeys, nonExistentKeys)
}
