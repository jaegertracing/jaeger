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
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/model"
	epmocks "github.com/jaegertracing/jaeger/plugin/sampling/leaderelection/mocks"
	"github.com/jaegertracing/jaeger/storage/samplingstore/mocks"
)

func TestAggregator(t *testing.T) {
	t.Skip("Skipping flaky unit test")
	metricsFactory := metricstest.NewFactory(0)

	mockStorage := &mocks.Store{}
	mockStorage.On("InsertThroughput", mock.AnythingOfType("[]*model.Throughput")).Return(nil)
	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("Start").Return(nil)
	mockEP.On("Close").Return(nil)
	mockEP.On("IsLeader").Return(true)
	testOpts := Options{
		CalculationInterval:   1 * time.Second,
		AggregationBuckets:    1,
		BucketsForCalculation: 1,
	}
	logger := zap.NewNop()

	a, err := NewAggregator(testOpts, logger, metricsFactory, mockEP, mockStorage)
	require.NoError(t, err)
	a.RecordThroughput("A", "GET", model.SamplerTypeProbabilistic, 0.001)
	a.RecordThroughput("B", "POST", model.SamplerTypeProbabilistic, 0.001)
	a.RecordThroughput("C", "GET", model.SamplerTypeProbabilistic, 0.001)
	a.RecordThroughput("A", "POST", model.SamplerTypeProbabilistic, 0.001)
	a.RecordThroughput("A", "GET", model.SamplerTypeProbabilistic, 0.001)
	a.RecordThroughput("A", "GET", model.SamplerTypeLowerBound, 0.001)

	a.Start()
	defer a.Close()
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
	mockEP := &epmocks.ElectionParticipant{}
	testOpts := Options{
		CalculationInterval:   1 * time.Second,
		AggregationBuckets:    1,
		BucketsForCalculation: 1,
	}
	logger := zap.NewNop()
	a, err := NewAggregator(testOpts, logger, metricsFactory, mockEP, mockStorage)
	require.NoError(t, err)
	// 20 different probabilities
	for i := 0; i < 20; i++ {
		a.RecordThroughput("A", "GET", model.SamplerTypeProbabilistic, 0.001*float64(i))
	}
	assert.Len(t, a.(*aggregator).currentThroughput["A"]["GET"].Probabilities, 10)

	a, err = NewAggregator(testOpts, logger, metricsFactory, mockEP, mockStorage)
	require.NoError(t, err)
	// 20 of the same probabilities
	for i := 0; i < 20; i++ {
		a.RecordThroughput("A", "GET", model.SamplerTypeProbabilistic, 0.001)
	}
	assert.Len(t, a.(*aggregator).currentThroughput["A"]["GET"].Probabilities, 1)
}

func TestLowerboundThroughput(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)
	mockStorage := &mocks.Store{}
	mockEP := &epmocks.ElectionParticipant{}
	testOpts := Options{
		CalculationInterval:   1 * time.Second,
		AggregationBuckets:    1,
		BucketsForCalculation: 1,
	}
	logger := zap.NewNop()

	a, err := NewAggregator(testOpts, logger, metricsFactory, mockEP, mockStorage)
	require.NoError(t, err)
	a.RecordThroughput("A", "GET", model.SamplerTypeLowerBound, 0.001)
	assert.EqualValues(t, 0, a.(*aggregator).currentThroughput["A"]["GET"].Count)
	assert.Empty(t, a.(*aggregator).currentThroughput["A"]["GET"].Probabilities["0.001000"])
}

func TestRecordThroughput(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)
	mockStorage := &mocks.Store{}
	mockEP := &epmocks.ElectionParticipant{}
	testOpts := Options{
		CalculationInterval:   1 * time.Second,
		AggregationBuckets:    1,
		BucketsForCalculation: 1,
	}
	logger := zap.NewNop()
	a, err := NewAggregator(testOpts, logger, metricsFactory, mockEP, mockStorage)
	require.NoError(t, err)

	// Testing non-root span
	span := &model.Span{References: []model.SpanRef{{SpanID: model.NewSpanID(1), RefType: model.ChildOf}}}
	a.HandleRootSpan(span, logger)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name but no operation
	span.References = []model.SpanRef{}
	span.Process = &model.Process{
		ServiceName: "A",
	}
	a.HandleRootSpan(span, logger)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name and operation but no probabilistic sampling tags
	span.OperationName = "GET"
	a.HandleRootSpan(span, logger)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name, operation, and probabilistic sampling tags
	span.Tags = model.KeyValues{
		model.String("sampler.type", "probabilistic"),
		model.String("sampler.param", "0.001"),
	}
	a.HandleRootSpan(span, logger)
	assert.EqualValues(t, 1, a.(*aggregator).currentThroughput["A"]["GET"].Count)
}

func TestRecordThroughputFunc(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)
	mockStorage := &mocks.Store{}
	mockEP := &epmocks.ElectionParticipant{}
	logger := zap.NewNop()
	testOpts := Options{
		CalculationInterval:   1 * time.Second,
		AggregationBuckets:    1,
		BucketsForCalculation: 1,
	}

	a, err := NewAggregator(testOpts, logger, metricsFactory, mockEP, mockStorage)
	require.NoError(t, err)

	// Testing non-root span
	span := &model.Span{References: []model.SpanRef{{SpanID: model.NewSpanID(1), RefType: model.ChildOf}}}
	RecordThroughput(a, span, logger)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name but no operation
	span.References = []model.SpanRef{}
	span.Process = &model.Process{
		ServiceName: "A",
	}
	RecordThroughput(a, span, logger)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name and operation but no probabilistic sampling tags
	span.OperationName = "GET"
	RecordThroughput(a, span, logger)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name, operation, and probabilistic sampling tags
	span.Tags = model.KeyValues{
		model.String("sampler.type", "probabilistic"),
		model.String("sampler.param", "0.001"),
	}
	RecordThroughput(a, span, logger)
	assert.EqualValues(t, 1, a.(*aggregator).currentThroughput["A"]["GET"].Count)
}
