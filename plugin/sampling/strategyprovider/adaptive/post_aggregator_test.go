// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	epmocks "github.com/jaegertracing/jaeger/internal/leaderelection/mocks"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive/calculationstrategy"
	smocks "github.com/jaegertracing/jaeger/storage/samplingstore/mocks"
	"github.com/jaegertracing/jaeger/storage/samplingstore/model"
)

func testThroughputs() []*model.Throughput {
	return []*model.Throughput{
		{Service: "svcA", Operation: http.MethodGet, Count: 4, Probabilities: map[string]struct{}{"0.1": {}}},
		{Service: "svcA", Operation: http.MethodGet, Count: 4, Probabilities: map[string]struct{}{"0.2": {}}},
		{Service: "svcA", Operation: http.MethodPut, Count: 5, Probabilities: map[string]struct{}{"0.1": {}}},
		{Service: "svcB", Operation: http.MethodGet, Count: 3, Probabilities: map[string]struct{}{"0.1": {}}},
	}
}

func testThroughputBuckets() []*throughputBucket {
	return []*throughputBucket{
		{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					http.MethodGet: {Count: 45},
					http.MethodPut: {Count: 60},
				},
				"svcB": map[string]*model.Throughput{
					http.MethodGet: {Count: 30},
					http.MethodPut: {Count: 15},
				},
			},
			interval: 60 * time.Second,
		},
		{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					http.MethodGet: {Count: 30},
				},
				"svcB": map[string]*model.Throughput{
					http.MethodGet: {Count: 45},
				},
			},
			interval: 60 * time.Second,
		},
	}
}

func errTestStorage() error {
	return errors.New("storage error")
}

func testCalculator() calculationstrategy.ProbabilityCalculator {
	return calculationstrategy.CalculateFunc(func(targetQPS, qps, oldProbability float64) float64 {
		factor := targetQPS / qps
		return oldProbability * factor
	})
}

func TestAggregateThroughputInputsImmutability(t *testing.T) {
	p := &PostAggregator{}
	in := testThroughputs()
	_ = p.aggregateThroughput(in)
	assert.Equal(t, in, testThroughputs())
}

func TestAggregateThroughput(t *testing.T) {
	p := &PostAggregator{}
	aggregatedThroughput := p.aggregateThroughput(testThroughputs())
	require.Len(t, aggregatedThroughput, 2)

	throughput, ok := aggregatedThroughput["svcA"]
	require.True(t, ok)
	require.Len(t, throughput, 2)

	opThroughput, ok := throughput[http.MethodGet]
	require.True(t, ok)
	assert.Equal(t, int64(8), opThroughput.Count)
	assert.Equal(t, map[string]struct{}{"0.1": {}, "0.2": {}}, opThroughput.Probabilities)

	opThroughput, ok = throughput[http.MethodPut]
	require.True(t, ok)
	assert.Equal(t, int64(5), opThroughput.Count)
	assert.Equal(t, map[string]struct{}{"0.1": {}}, opThroughput.Probabilities)

	throughput, ok = aggregatedThroughput["svcB"]
	require.True(t, ok)
	require.Len(t, throughput, 1)

	opThroughput, ok = throughput[http.MethodGet]
	require.True(t, ok)
	assert.Equal(t, int64(3), opThroughput.Count)
	assert.Equal(t, map[string]struct{}{"0.1": {}}, opThroughput.Probabilities)
}

func TestInitializeThroughput(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*19), time.Time{}.Add(time.Minute*20)).
		Return(testThroughputs(), nil)
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*18), time.Time{}.Add(time.Minute*19)).
		Return([]*model.Throughput{{Service: "svcA", Operation: http.MethodGet, Count: 7}}, nil)
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*17), time.Time{}.Add(time.Minute*18)).
		Return([]*model.Throughput{}, nil)
	p := &PostAggregator{storage: mockStorage, Options: Options{CalculationInterval: time.Minute, AggregationBuckets: 3}}
	p.initializeThroughput(time.Time{}.Add(time.Minute * 20))

	require.Len(t, p.throughputs, 2)
	require.Len(t, p.throughputs[0].throughput, 2)
	assert.Equal(t, time.Minute, p.throughputs[0].interval)
	assert.Equal(t, p.throughputs[0].endTime, time.Time{}.Add(time.Minute*20))
	require.Len(t, p.throughputs[1].throughput, 1)
	assert.Equal(t, time.Minute, p.throughputs[1].interval)
	assert.Equal(t, p.throughputs[1].endTime, time.Time{}.Add(time.Minute*19))
}

func TestInitializeThroughputFailure(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*19), time.Time{}.Add(time.Minute*20)).
		Return(nil, errTestStorage())
	p := &PostAggregator{storage: mockStorage, Options: Options{CalculationInterval: time.Minute, AggregationBuckets: 1}}
	p.initializeThroughput(time.Time{}.Add(time.Minute * 20))

	assert.Empty(t, p.throughputs)
}

func TestCalculateQPS(t *testing.T) {
	qps := calculateQPS(int64(90), 60*time.Second)
	assert.InDelta(t, 1.5, qps, 0.01)

	qps = calculateQPS(int64(45), 60*time.Second)
	assert.InDelta(t, 0.75, qps, 0.01)
}

func TestGenerateOperationQPS(t *testing.T) {
	p := &PostAggregator{throughputs: testThroughputBuckets(), Options: Options{BucketsForCalculation: 10, AggregationBuckets: 10}}
	svcOpQPS := p.throughputToQPS()
	assert.Len(t, svcOpQPS, 2)

	opQPS, ok := svcOpQPS["svcA"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.75, 0.5}, opQPS[http.MethodGet])
	assert.Equal(t, []float64{1.0}, opQPS[http.MethodPut])

	opQPS, ok = svcOpQPS["svcB"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.5, 0.75}, opQPS[http.MethodGet])
	assert.Equal(t, []float64{0.25}, opQPS[http.MethodPut])

	// Test using the previous QPS if the throughput is not provided
	p.prependThroughputBucket(
		&throughputBucket{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					http.MethodGet: {Count: 30},
				},
			},
			interval: 60 * time.Second,
		},
	)
	svcOpQPS = p.throughputToQPS()
	require.Len(t, svcOpQPS, 2)

	opQPS, ok = svcOpQPS["svcA"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.5, 0.75, 0.5}, opQPS[http.MethodGet])
	assert.Equal(t, []float64{1.0}, opQPS[http.MethodPut])

	opQPS, ok = svcOpQPS["svcB"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.5, 0.75}, opQPS[http.MethodGet])
	assert.Equal(t, []float64{0.25}, opQPS[http.MethodPut])
}

func TestGenerateOperationQPS_UseMostRecentBucketOnly(t *testing.T) {
	p := &PostAggregator{throughputs: testThroughputBuckets(), Options: Options{BucketsForCalculation: 1, AggregationBuckets: 10}}
	svcOpQPS := p.throughputToQPS()
	assert.Len(t, svcOpQPS, 2)

	opQPS, ok := svcOpQPS["svcA"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.75}, opQPS[http.MethodGet])
	assert.Equal(t, []float64{1.0}, opQPS[http.MethodPut])

	p.prependThroughputBucket(
		&throughputBucket{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					http.MethodGet: {Count: 30},
				},
			},
			interval: 60 * time.Second,
		},
	)

	svcOpQPS = p.throughputToQPS()
	require.Len(t, svcOpQPS, 2)

	opQPS, ok = svcOpQPS["svcA"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.5}, opQPS[http.MethodGet])
	assert.Equal(t, []float64{1.0}, opQPS[http.MethodPut])
}

func TestCalculateWeightedQPS(t *testing.T) {
	p := PostAggregator{weightVectorCache: NewWeightVectorCache()}
	assert.InDelta(t, 0.86735, p.calculateWeightedQPS([]float64{0.8, 1.2, 1.0}), 0.001)
	assert.InDelta(t, 0.95197, p.calculateWeightedQPS([]float64{1.0, 1.0, 0.0, 0.0}), 0.001)
	assert.InDelta(t, 0.0, p.calculateWeightedQPS([]float64{}), 0.01)
}

func TestCalculateProbability(t *testing.T) {
	throughputs := []*throughputBucket{
		{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					http.MethodGet: {Probabilities: map[string]struct{}{"0.500000": {}}},
				},
			},
		},
	}
	probabilities := model.ServiceOperationProbabilities{
		"svcA": map[string]float64{
			http.MethodGet: 0.5,
		},
	}
	cfg := Options{
		TargetSamplesPerSecond:     1.0,
		DeltaTolerance:             0.2,
		InitialSamplingProbability: 0.001,
		MinSamplingProbability:     0.00001,
	}
	p := &PostAggregator{
		Options:               cfg,
		probabilities:         probabilities,
		probabilityCalculator: testCalculator(),
		throughputs:           throughputs,
		serviceCache:          []SamplingCache{{"svcA": {}, "svcB": {}}},
	}
	tests := []struct {
		service             string
		operation           string
		qps                 float64
		expectedProbability float64
		errMsg              string
	}{
		{"svcA", http.MethodGet, 2.0, 0.25, "modify existing probability"},
		{"svcA", http.MethodPut, 2.0, 0.0005, "modify default probability"},
		{"svcB", http.MethodGet, 0.9, 0.001, "qps within equivalence threshold"},
		{"svcB", http.MethodPut, 0.000001, 1.0, "test max probability"},
		{"svcB", http.MethodDelete, 1000000000, 0.00001, "test min probability"},
		{"svcB", http.MethodDelete, 0.0, 0.002, "test 0 qps"},
	}
	for _, test := range tests {
		probability := p.calculateProbability(test.service, test.operation, test.qps)
		assert.InDelta(t, test.expectedProbability, probability, 1e-6, test.errMsg)
	}
}

func TestCalculateProbabilitiesAndQPS(t *testing.T) {
	prevProbabilities := model.ServiceOperationProbabilities{
		"svcB": map[string]float64{
			http.MethodGet: 0.16,
			http.MethodPut: 0.03,
		},
	}
	qps := model.ServiceOperationQPS{
		"svcB": map[string]float64{
			http.MethodGet: 0.625,
		},
	}
	mets := metricstest.NewFactory(0)
	p := &PostAggregator{
		Options: Options{
			TargetSamplesPerSecond:     1.0,
			DeltaTolerance:             0.2,
			InitialSamplingProbability: 0.001,
			BucketsForCalculation:      10,
		},
		throughputs: testThroughputBuckets(), probabilities: prevProbabilities, qps: qps,
		weightVectorCache: NewWeightVectorCache(), probabilityCalculator: testCalculator(),
		operationsCalculatedGauge: mets.Gauge(metrics.Options{Name: "test"}),
	}
	probabilities, qps := p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.00136, http.MethodPut: 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodGet: 0.16, http.MethodPut: 0.03}, probabilities["svcB"])

	require.Len(t, qps, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.7352941176470588, http.MethodPut: 1}, qps["svcA"])
	assert.Equal(t, map[string]float64{http.MethodGet: 0.5147058823529411, http.MethodPut: 0.25}, qps["svcB"])

	_, gauges := mets.Backend.Snapshot()
	assert.EqualValues(t, 4, gauges["test"])
}

func TestRunCalculationLoop(t *testing.T) {
	logger := zap.NewNop()
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return(testThroughputs(), nil)
	mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{}, errTestStorage())
	mockStorage.On("InsertProbabilitiesAndQPS", mock.AnythingOfType("string"), mock.AnythingOfType("model.ServiceOperationProbabilities"),
		mock.AnythingOfType("model.ServiceOperationQPS")).Return(errTestStorage())
	mockStorage.On("InsertThroughput", mock.AnythingOfType("[]*model.Throughput")).Return(errTestStorage())
	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("Start").Return(nil)
	mockEP.On("Close").Return(nil)
	mockEP.On("IsLeader").Return(true)

	cfg := Options{
		TargetSamplesPerSecond:       1.0,
		DeltaTolerance:               0.1,
		InitialSamplingProbability:   0.001,
		CalculationInterval:          time.Millisecond * 5,
		AggregationBuckets:           2,
		Delay:                        time.Millisecond * 5,
		LeaderLeaseRefreshInterval:   time.Millisecond,
		FollowerLeaseRefreshInterval: time.Second,
		BucketsForCalculation:        10,
	}
	agg, err := NewAggregator(cfg, logger, metrics.NullFactory, mockEP, mockStorage)
	require.NoError(t, err)
	agg.Start()
	defer agg.Close()

	for i := 0; i < 1000; i++ {
		agg.(*aggregator).Lock()
		probabilities := agg.(*aggregator).postAggregator.probabilities
		agg.(*aggregator).Unlock()
		if len(probabilities) != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	postAgg := agg.(*aggregator).postAggregator
	postAgg.Lock()
	probabilities := postAgg.probabilities
	postAgg.Unlock()
	require.Len(t, probabilities["svcA"], 2)
}

func TestRunCalculationLoop_GetThroughputError(t *testing.T) {
	logger, logBuffer := testutils.NewLogger()
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return(nil, errTestStorage())
	mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{}, errTestStorage())
	mockStorage.On("InsertProbabilitiesAndQPS", mock.AnythingOfType("string"), mock.AnythingOfType("model.ServiceOperationProbabilities"),
		mock.AnythingOfType("model.ServiceOperationQPS")).Return(errTestStorage())
	mockStorage.On("InsertThroughput", mock.AnythingOfType("[]*model.Throughput")).Return(errTestStorage())

	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("Start").Return(nil)
	mockEP.On("Close").Return(nil)
	mockEP.On("IsLeader").Return(false)

	cfg := Options{
		CalculationInterval:   time.Millisecond * 5,
		AggregationBuckets:    2,
		BucketsForCalculation: 10,
	}
	agg, err := NewAggregator(cfg, logger, metrics.NullFactory, mockEP, mockStorage)
	require.NoError(t, err)
	agg.Start()
	for i := 0; i < 1000; i++ {
		// match logs specific to getThroughputErrMsg. We expect to see more than 2, once during
		// initialization and one or more times during the loop.
		if match, _ := testutils.LogMatcher(2, getThroughputErrMsg, logBuffer.Lines()); match {
			break
		}
		time.Sleep(time.Millisecond)
	}
	match, errMsg := testutils.LogMatcher(2, getThroughputErrMsg, logBuffer.Lines())
	assert.True(t, match, errMsg)
	require.NoError(t, agg.Close())
}

func TestPrependBucket(t *testing.T) {
	p := &PostAggregator{Options: Options{AggregationBuckets: 1}}
	p.prependThroughputBucket(&throughputBucket{interval: time.Minute})
	require.Len(t, p.throughputs, 1)
	assert.Equal(t, time.Minute, p.throughputs[0].interval)

	p.prependThroughputBucket(&throughputBucket{interval: 2 * time.Minute})
	require.Len(t, p.throughputs, 1)
	assert.Equal(t, 2*time.Minute, p.throughputs[0].interval)
}

func TestConstructorFailure(t *testing.T) {
	logger := zap.NewNop()

	cfg := Options{
		TargetSamplesPerSecond:     1.0,
		DeltaTolerance:             0.2,
		InitialSamplingProbability: 0.001,
		CalculationInterval:        time.Second * 5,
		AggregationBuckets:         0,
	}
	_, err := newPostAggregator(cfg, "host", nil, nil, metrics.NullFactory, logger)
	require.EqualError(t, err, "CalculationInterval and AggregationBuckets must be greater than 0")

	cfg.CalculationInterval = 0
	_, err = newPostAggregator(cfg, "host", nil, nil, metrics.NullFactory, logger)
	require.EqualError(t, err, "CalculationInterval and AggregationBuckets must be greater than 0")

	cfg.CalculationInterval = time.Millisecond
	cfg.AggregationBuckets = 1
	cfg.BucketsForCalculation = -1
	_, err = newPostAggregator(cfg, "host", nil, nil, metrics.NullFactory, logger)
	require.EqualError(t, err, "BucketsForCalculation cannot be less than 1")
}

func TestUsingAdaptiveSampling(t *testing.T) {
	p := &PostAggregator{}
	throughput := serviceOperationThroughput{
		"svc": map[string]*model.Throughput{
			"op": {Probabilities: map[string]struct{}{"0.010000": {}}},
		},
	}
	tests := []struct {
		expected    bool
		probability float64
		service     string
		operation   string
	}{
		{expected: true, probability: 0.01, service: "svc", operation: "op"},
		{expected: true, probability: 0.0099999384, service: "svc", operation: "op"},
		{expected: false, probability: 0.01, service: "non-svc"},
		{expected: false, probability: 0.01, service: "svc", operation: "non-op"},
		{expected: false, probability: 0.01, service: "svc", operation: "non-op"},
		{expected: false, probability: 0.02, service: "svc", operation: "op"},
		{expected: false, probability: 0.0100009384, service: "svc", operation: "op"},
	}
	for _, test := range tests {
		assert.Equal(t, test.expected, p.isUsingAdaptiveSampling(test.probability, test.service, test.operation, throughput))
	}
}

func TestPrependServiceCache(t *testing.T) {
	p := &PostAggregator{}
	for i := 0; i < serviceCacheSize*2; i++ {
		p.prependServiceCache()
	}
	assert.Len(t, p.serviceCache, serviceCacheSize)
}

func TestCalculateProbabilitiesAndQPSMultiple(t *testing.T) {
	buckets := []*throughputBucket{
		{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					http.MethodGet: {Count: 3, Probabilities: map[string]struct{}{"0.001000": {}}},
					http.MethodPut: {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
				},
				"svcB": map[string]*model.Throughput{
					http.MethodPut: {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
				},
			},
			interval: 60 * time.Second,
		},
	}

	p := &PostAggregator{
		Options: Options{
			TargetSamplesPerSecond:     1.0,
			DeltaTolerance:             0.002,
			InitialSamplingProbability: 0.001,
			BucketsForCalculation:      5,
			AggregationBuckets:         10,
		},
		throughputs: buckets, probabilities: make(model.ServiceOperationProbabilities),
		qps: make(model.ServiceOperationQPS), weightVectorCache: NewWeightVectorCache(),
		probabilityCalculator:     calculationstrategy.NewPercentageIncreaseCappedCalculator(1.0),
		serviceCache:              []SamplingCache{},
		operationsCalculatedGauge: metrics.NullFactory.Gauge(metrics.Options{}),
	}

	probabilities, qps := p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.002, http.MethodPut: 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET is no longer reported, we should not increase it's probability since we don't know if it's adaptively sampled
	// until we get at least a lowerbound span or a probability span with the right probability.
	// svcB:PUT is only reporting lowerbound, we should boost it's probability
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodPut: {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodGet: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				http.MethodPut: {Count: 0, Probabilities: map[string]struct{}{"0.002000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.002, http.MethodPut: 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.004, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET is lower bound sampled, increase its probability
	// svcB:PUT is not reported but we should boost it's probability since the previous calculation showed that
	// it's using adaptive sampling
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodGet: {Count: 0, Probabilities: map[string]struct{}{"0.002000": {}}},
				http.MethodPut: {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodGet: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.004, http.MethodPut: 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.008, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET is finally adaptively probabilistically sampled!
	// svcB:PUT stopped using adaptive sampling
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodGet: {Count: 1, Probabilities: map[string]struct{}{"0.004000": {}}},
				http.MethodPut: {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodGet: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				http.MethodPut: {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.008, http.MethodPut: 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.008, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodPut: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodGet: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				http.MethodPut: {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.016, http.MethodPut: 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.008, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT starts to use adaptive sampling again
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodPut: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodGet: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				http.MethodPut: {Count: 1, Probabilities: map[string]struct{}{"0.008000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.032, http.MethodPut: 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.016, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodPut: {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodGet: {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.064, http.MethodPut: 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.032, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodPut: {Count: 20, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodGet: {Count: 10, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.128, http.MethodPut: 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.064, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				http.MethodPut: {Count: 20, Probabilities: map[string]struct{}{"0.001000": {}}},
				http.MethodGet: {Count: 120, Probabilities: map[string]struct{}{"0.128000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				http.MethodPut: {Count: 60, Probabilities: map[string]struct{}{"0.064000": {}}},
				http.MethodGet: {Count: 10, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{http.MethodGet: 0.0882586677054928, http.MethodPut: 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{http.MethodPut: 0.09587513707888091, http.MethodGet: 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps
}
