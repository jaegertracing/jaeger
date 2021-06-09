// Copyright (c) 2021 The Jaeger Authors.
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

package adaptive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics/metricstest"

	"github.com/jaegertracing/jaeger/storage/samplingstore/mocks"
)

const (
	probabilistic = "probabilistic"
	lowerbound    = "lowerbound"
)

func TestAggregator(t *testing.T) {
	t.Skip("Skipping flaky unit test")
	metricsFactory := metricstest.NewFactory(0)

	mockStorage := &mocks.Store{}
	mockStorage.On("InsertThroughput", mock.AnythingOfType("[]*model.Throughput")).Return(nil)

	a := NewAggregator(metricsFactory, 5*time.Millisecond, mockStorage)
	a.RecordThroughput("A", "GET", probabilistic, 0.001)
	a.RecordThroughput("B", "POST", probabilistic, 0.001)
	a.RecordThroughput("C", "GET", probabilistic, 0.001)
	a.RecordThroughput("A", "POST", probabilistic, 0.001)
	a.RecordThroughput("A", "GET", probabilistic, 0.001)
	a.RecordThroughput("A", "GET", lowerbound, 0.001)

	a.Start()
	defer a.Stop()
	for i := 0; i < 10000; i++ {
		counters, _ := metricsFactory.Snapshot()
		if _, ok := counters["sampling_operations"]; ok {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	metricsFactory.AssertCounterMetrics(t, []metricstest.ExpectedMetric{
		{Name: "sampling_operations", Value: 4},
		{Name: "sampling_services", Value: 3},
	}...)
}

func TestIncrementThroughput(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)
	mockStorage := &mocks.Store{}

	a := NewAggregator(metricsFactory, 5*time.Millisecond, mockStorage)
	// 20 different probabilities
	for i := 0; i < 20; i++ {
		a.RecordThroughput("A", "GET", probabilistic, 0.001*float64(i))
	}
	assert.Len(t, a.(*aggregator).currentThroughput["A"]["GET"].Probabilities, 10)

	a = NewAggregator(metricsFactory, 5*time.Millisecond, mockStorage)
	// 20 of the same probabilities
	for i := 0; i < 20; i++ {
		a.RecordThroughput("A", "GET", probabilistic, 0.001)
	}
	assert.Len(t, a.(*aggregator).currentThroughput["A"]["GET"].Probabilities, 1)
}

func TestLowerboundThroughput(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)
	mockStorage := &mocks.Store{}

	a := NewAggregator(metricsFactory, 5*time.Millisecond, mockStorage)
	a.RecordThroughput("A", "GET", lowerbound, 0.001)
	assert.EqualValues(t, 0, a.(*aggregator).currentThroughput["A"]["GET"].Count)
	assert.Empty(t, a.(*aggregator).currentThroughput["A"]["GET"].Probabilities["0.001000"])
}
