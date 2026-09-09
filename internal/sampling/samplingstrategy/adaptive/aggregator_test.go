// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"net/http"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	epmocks "github.com/jaegertracing/jaeger/internal/leaderelection/mocks"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/mocks"
)

func TestAggregator(t *testing.T) {
	// synctest gives the aggregation loop a fake clock, so the tick is triggered
	// deterministically instead of waiting for one and hoping it landed.
	synctest.Test(t, func(t *testing.T) {
		metricsFactory := metricstest.NewFactory(0)

		mockStorage := &mocks.Store{}
		mockStorage.On("InsertThroughput", mock.AnythingOfType("[]*model.Throughput")).Return(nil)
		// Start() initializes the post-aggregator's throughput buckets, and its
		// calculation loop saves probabilities back once this host is leader. Both
		// read through the store, so both need stubbing or the mock panics.
		mockStorage.On("GetThroughput", mock.Anything, mock.Anything).Return(nil, nil)
		mockStorage.On("InsertProbabilitiesAndQPS", mock.Anything, mock.Anything, mock.Anything).Return(nil)
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
		a.RecordThroughput("A", http.MethodGet, model.SamplerTypeProbabilistic, 0.001)
		a.RecordThroughput("B", http.MethodPost, model.SamplerTypeProbabilistic, 0.001)
		a.RecordThroughput("C", http.MethodGet, model.SamplerTypeProbabilistic, 0.001)
		a.RecordThroughput("A", http.MethodPost, model.SamplerTypeProbabilistic, 0.001)
		a.RecordThroughput("A", http.MethodGet, model.SamplerTypeProbabilistic, 0.001)
		a.RecordThroughput("A", http.MethodGet, model.SamplerTypeLowerBound, 0.001)

		a.Start()
		defer a.Close()

		// Advance past exactly one aggregation tick. Inside the bubble this is
		// instant, and synctest.Wait blocks until the loop goroutine has finished
		// handling the tick, so the counters below cannot be read mid-update.
		time.Sleep(testOpts.CalculationInterval + time.Millisecond)
		synctest.Wait()

		metricsFactory.AssertCounterMetrics(t, []metricstest.ExpectedMetric{
			{Name: "sampling_operations", Value: 4},
			{Name: "sampling_services", Value: 3},
		}...)
	})
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
	for i := range 20 {
		a.RecordThroughput("A", http.MethodGet, model.SamplerTypeProbabilistic, 0.001*float64(i))
	}
	assert.Len(t, a.(*aggregator).currentThroughput["A"][http.MethodGet].Probabilities, 10)

	a, err = NewAggregator(testOpts, logger, metricsFactory, mockEP, mockStorage)
	require.NoError(t, err)
	// 20 of the same probabilities
	for range 20 {
		a.RecordThroughput("A", http.MethodGet, model.SamplerTypeProbabilistic, 0.001)
	}
	assert.Len(t, a.(*aggregator).currentThroughput["A"][http.MethodGet].Probabilities, 1)
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
	a.RecordThroughput("A", http.MethodGet, model.SamplerTypeLowerBound, 0.001)
	assert.EqualValues(t, 0, a.(*aggregator).currentThroughput["A"][http.MethodGet].Count)
	assert.Empty(t, a.(*aggregator).currentThroughput["A"][http.MethodGet].Probabilities["0.001000"])
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
	a.HandleRootSpan(span)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name but no operation
	span.References = []model.SpanRef{}
	span.Process = &model.Process{
		ServiceName: "A",
	}
	a.HandleRootSpan(span)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name and operation but no probabilistic sampling tags
	span.OperationName = http.MethodGet
	a.HandleRootSpan(span)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name, operation, and probabilistic sampling tags
	span.Tags = model.KeyValues{
		model.String("sampler.type", "probabilistic"),
		model.String("sampler.param", "0.001"),
	}
	a.HandleRootSpan(span)
	assert.EqualValues(t, 1, a.(*aggregator).currentThroughput["A"][http.MethodGet].Count)
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
	a.HandleRootSpan(span)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name but no operation
	span.References = []model.SpanRef{}
	span.Process = &model.Process{
		ServiceName: "A",
	}
	a.HandleRootSpan(span)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name and operation but no probabilistic sampling tags
	span.OperationName = http.MethodGet
	a.HandleRootSpan(span)
	require.Empty(t, a.(*aggregator).currentThroughput)

	// Testing span with service name, operation, and probabilistic sampling tags
	span.Tags = model.KeyValues{
		model.String("sampler.type", "probabilistic"),
		model.String("sampler.param", "0.001"),
	}
	a.HandleRootSpan(span)
	assert.EqualValues(t, 1, a.(*aggregator).currentThroughput["A"][http.MethodGet].Count)
}

func TestGetSamplerParams(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		tags          model.KeyValues
		expectedType  model.SamplerType
		expectedParam float64
	}{
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.String("sampler.param", "1e-05"),
			},
			expectedType:  model.SamplerTypeProbabilistic,
			expectedParam: 0.00001,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.Float64("sampler.param", 0.10404450002098709),
			},
			expectedType:  model.SamplerTypeProbabilistic,
			expectedParam: 0.10404450002098709,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.String("sampler.param", "0.10404450002098709"),
			},
			expectedType:  model.SamplerTypeProbabilistic,
			expectedParam: 0.10404450002098709,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.Int64("sampler.param", 1),
			},
			expectedType:  model.SamplerTypeProbabilistic,
			expectedParam: 1.0,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "ratelimiting"),
				model.String("sampler.param", "1"),
			},
			expectedType:  model.SamplerTypeRateLimiting,
			expectedParam: 1,
		},
		{
			tags: model.KeyValues{
				model.Float64("sampler.type", 1.5),
			},
			expectedType:  model.SamplerTypeUnrecognized,
			expectedParam: 0,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
			},
			expectedType:  model.SamplerTypeUnrecognized,
			expectedParam: 0,
		},
		{
			tags:          model.KeyValues{},
			expectedType:  model.SamplerTypeUnrecognized,
			expectedParam: 0,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.String("sampler.param", "1"),
			},
			expectedType:  model.SamplerTypeLowerBound,
			expectedParam: 1,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.Int64("sampler.param", 1),
			},
			expectedType:  model.SamplerTypeLowerBound,
			expectedParam: 1,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.Float64("sampler.param", 0.5),
			},
			expectedType:  model.SamplerTypeLowerBound,
			expectedParam: 0.5,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.String("sampler.param", "not_a_number"),
			},
			expectedType:  model.SamplerTypeUnrecognized,
			expectedParam: 0,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "not_a_type"),
				model.String("sampler.param", "not_a_number"),
			},
			expectedType:  model.SamplerTypeUnrecognized,
			expectedParam: 0,
		},
	}

	for i, test := range tests {
		tt := test
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			span := &model.Span{}
			span.Tags = tt.tags
			actualType, actualParam := getSamplerParams(span, logger)
			assert.Equal(t, tt.expectedType, actualType)
			assert.InDelta(t, tt.expectedParam, actualParam, 0.01)
		})
	}
}
