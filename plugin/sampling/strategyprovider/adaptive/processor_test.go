// Copyright (c) 2018 The Jaeger Authors.
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
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	epmocks "github.com/jaegertracing/jaeger/plugin/sampling/leaderelection/mocks"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive/calculationstrategy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	smocks "github.com/jaegertracing/jaeger/storage/samplingstore/mocks"
)

func testThroughputs() []*model.Throughput {
	return []*model.Throughput{
		{Service: "svcA", Operation: "GET", Count: 4, Probabilities: map[string]struct{}{"0.1": {}}},
		{Service: "svcA", Operation: "GET", Count: 4, Probabilities: map[string]struct{}{"0.2": {}}},
		{Service: "svcA", Operation: "PUT", Count: 5, Probabilities: map[string]struct{}{"0.1": {}}},
		{Service: "svcB", Operation: "GET", Count: 3, Probabilities: map[string]struct{}{"0.1": {}}},
	}
}

func testThroughputBuckets() []*throughputBucket {
	return []*throughputBucket{
		{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					"GET": {Count: 45},
					"PUT": {Count: 60},
				},
				"svcB": map[string]*model.Throughput{
					"GET": {Count: 30},
					"PUT": {Count: 15},
				},
			},
			interval: 60 * time.Second,
		},
		{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					"GET": {Count: 30},
				},
				"svcB": map[string]*model.Throughput{
					"GET": {Count: 45},
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

	opThroughput, ok := throughput["GET"]
	require.True(t, ok)
	assert.Equal(t, int64(8), opThroughput.Count)
	assert.Equal(t, map[string]struct{}{"0.1": {}, "0.2": {}}, opThroughput.Probabilities)

	opThroughput, ok = throughput["PUT"]
	require.True(t, ok)
	assert.Equal(t, int64(5), opThroughput.Count)
	assert.Equal(t, map[string]struct{}{"0.1": {}}, opThroughput.Probabilities)

	throughput, ok = aggregatedThroughput["svcB"]
	require.True(t, ok)
	require.Len(t, throughput, 1)

	opThroughput, ok = throughput["GET"]
	require.True(t, ok)
	assert.Equal(t, int64(3), opThroughput.Count)
	assert.Equal(t, map[string]struct{}{"0.1": {}}, opThroughput.Probabilities)
}

func TestInitializeThroughput(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*19), time.Time{}.Add(time.Minute*20)).
		Return(testThroughputs(), nil)
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*18), time.Time{}.Add(time.Minute*19)).
		Return([]*model.Throughput{{Service: "svcA", Operation: "GET", Count: 7}}, nil)
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*17), time.Time{}.Add(time.Minute*18)).
		Return([]*model.Throughput{}, nil)
	p := &PostAggregator{
		storage:            mockStorage,
		AggregatorSettings: AggregatorSettings{CalculationInterval: time.Minute, AggregationBuckets: 3}}
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
	p := &PostAggregator{storage: mockStorage, AggregatorSettings: AggregatorSettings{CalculationInterval: time.Minute, AggregationBuckets: 1}}
	p.initializeThroughput(time.Time{}.Add(time.Minute * 20))

	assert.Empty(t, p.throughputs)
}

func TestCalculateQPS(t *testing.T) {
	qps := calculateQPS(int64(90), 60*time.Second)
	assert.Equal(t, 1.5, qps)

	qps = calculateQPS(int64(45), 60*time.Second)
	assert.Equal(t, 0.75, qps)
}

func TestGenerateOperationQPS(t *testing.T) {
	p := &PostAggregator{throughputs: testThroughputBuckets(), AggregatorSettings: AggregatorSettings{BucketsForCalculation: 10, AggregationBuckets: 10}}
	svcOpQPS := p.throughputToQPS()
	assert.Len(t, svcOpQPS, 2)

	opQPS, ok := svcOpQPS["svcA"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.75, 0.5}, opQPS["GET"])
	assert.Equal(t, []float64{1.0}, opQPS["PUT"])

	opQPS, ok = svcOpQPS["svcB"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.5, 0.75}, opQPS["GET"])
	assert.Equal(t, []float64{0.25}, opQPS["PUT"])

	// Test using the previous QPS if the throughput is not provided
	p.prependThroughputBucket(
		&throughputBucket{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					"GET": {Count: 30},
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

	assert.Equal(t, []float64{0.5, 0.75, 0.5}, opQPS["GET"])
	assert.Equal(t, []float64{1.0}, opQPS["PUT"])

	opQPS, ok = svcOpQPS["svcB"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.5, 0.75}, opQPS["GET"])
	assert.Equal(t, []float64{0.25}, opQPS["PUT"])
}

func TestGenerateOperationQPS_UseMostRecentBucketOnly(t *testing.T) {
	p := &PostAggregator{throughputs: testThroughputBuckets(), AggregatorSettings: AggregatorSettings{BucketsForCalculation: 1, AggregationBuckets: 10}}
	svcOpQPS := p.throughputToQPS()
	assert.Len(t, svcOpQPS, 2)

	opQPS, ok := svcOpQPS["svcA"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.75}, opQPS["GET"])
	assert.Equal(t, []float64{1.0}, opQPS["PUT"])

	p.prependThroughputBucket(
		&throughputBucket{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					"GET": {Count: 30},
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

	assert.Equal(t, []float64{0.5}, opQPS["GET"])
	assert.Equal(t, []float64{1.0}, opQPS["PUT"])
}

func TestCalculateWeightedQPS(t *testing.T) {
	p := PostAggregator{weightVectorCache: NewWeightVectorCache()}
	assert.InDelta(t, 0.86735, p.calculateWeightedQPS([]float64{0.8, 1.2, 1.0}), 0.001)
	assert.InDelta(t, 0.95197, p.calculateWeightedQPS([]float64{1.0, 1.0, 0.0, 0.0}), 0.001)
	assert.Equal(t, 0.0, p.calculateWeightedQPS([]float64{}))
}

func TestCalculateProbability(t *testing.T) {
	throughputs := []*throughputBucket{
		{
			throughput: serviceOperationThroughput{
				"svcA": map[string]*model.Throughput{
					"GET": {Probabilities: map[string]struct{}{"0.500000": {}}},
				},
			},
		},
	}
	probabilities := model.ServiceOperationProbabilities{
		"svcA": map[string]float64{
			"GET": 0.5,
		},
	}
	cfg := AggregatorSettings{
		TargetSamplesPerSecond:     1.0,
		DeltaTolerance:             0.2,
		InitialSamplingProbability: 0.001,
		MinSamplingProbability:     0.00001,
	}
	p := &PostAggregator{
		AggregatorSettings:    cfg,
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
		{"svcA", "GET", 2.0, 0.25, "modify existing probability"},
		{"svcA", "PUT", 2.0, 0.0005, "modify default probability"},
		{"svcB", "GET", 0.9, 0.001, "qps within equivalence threshold"},
		{"svcB", "PUT", 0.000001, 1.0, "test max probability"},
		{"svcB", "DELETE", 1000000000, 0.00001, "test min probability"},
		{"svcB", "DELETE", 0.0, 0.002, "test 0 qps"},
	}
	for _, test := range tests {
		probability := p.calculateProbability(test.service, test.operation, test.qps)
		assert.Equal(t, test.expectedProbability, probability, test.errMsg)
	}
}

func TestCalculateProbabilitiesAndQPS(t *testing.T) {
	prevProbabilities := model.ServiceOperationProbabilities{
		"svcB": map[string]float64{
			"GET": 0.16,
			"PUT": 0.03,
		},
	}
	qps := model.ServiceOperationQPS{
		"svcB": map[string]float64{
			"GET": 0.625,
		},
	}
	mets := metricstest.NewFactory(0)
	p := &PostAggregator{
		AggregatorSettings: AggregatorSettings{
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
	assert.Equal(t, map[string]float64{"GET": 0.00136, "PUT": 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"GET": 0.16, "PUT": 0.03}, probabilities["svcB"])

	require.Len(t, qps, 2)
	assert.Equal(t, map[string]float64{"GET": 0.7352941176470588, "PUT": 1}, qps["svcA"])
	assert.Equal(t, map[string]float64{"GET": 0.5147058823529411, "PUT": 0.25}, qps["svcB"])

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

	cfg := AggregatorSettings{
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
	agg, err := NewAggregator(cfg, mockEP, mockStorage, logger, metrics.NullFactory)
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

	cfg := AggregatorSettings{
		CalculationInterval:   time.Millisecond * 5,
		AggregationBuckets:    2,
		BucketsForCalculation: 10,
	}
	agg, err := NewAggregator(cfg, mockEP, mockStorage, logger, metrics.NullFactory)
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

func TestLoadProbabilities(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)

	p := &Provider{storage: mockStorage}
	require.Nil(t, p.probabilities)
	p.loadProbabilities()
	require.NotNil(t, p.probabilities)
}

func TestRunUpdateProbabilitiesLoop(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)
	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("Start").Return(nil)
	mockEP.On("Close").Return(nil)
	mockEP.On("IsLeader").Return(false)

	p := &Provider{
		storage:                 mockStorage,
		shutdown:                make(chan struct{}),
		followerRefreshInterval: time.Millisecond,
		electionParticipant:     mockEP,
	}
	defer close(p.shutdown)
	require.Nil(t, p.probabilities)
	require.Nil(t, p.strategyResponses)
	go p.runUpdateProbabilitiesLoop()

	for i := 0; i < 1000; i++ {
		p.RLock()
		if p.probabilities != nil && p.strategyResponses != nil {
			p.RUnlock()
			break
		}
		p.RUnlock()
		time.Sleep(time.Millisecond)
	}
	p.RLock()
	assert.NotNil(t, p.probabilities)
	assert.NotNil(t, p.strategyResponses)
	p.RUnlock()
}

func TestRealisticRunCalculationLoop(t *testing.T) {
	t.Skip("Skipped realistic calculation loop test")
	logger := zap.NewNop()
	// NB: This is an extremely long test since it uses near realistic (1/6th scale) processor config values
	testThroughputs := []*model.Throughput{
		{Service: "svcA", Operation: "GET", Count: 10},
		{Service: "svcA", Operation: "POST", Count: 9},
		{Service: "svcA", Operation: "PUT", Count: 5},
		{Service: "svcA", Operation: "DELETE", Count: 20},
	}
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return(testThroughputs, nil)
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)
	mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.AnythingOfType("model.ServiceOperationProbabilities"),
		mock.AnythingOfType("model.ServiceOperationQPS")).Return(nil)
	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("Start").Return(nil)
	mockEP.On("Close").Return(nil)
	mockEP.On("IsLeader").Return(true)
	cfg := ProviderSettings{
		// TargetSamplesPerSecond:     1.0,
		// DeltaTolerance:             0.2,
		InitialSamplingProbability: 0.001,
		// CalculationInterval:        time.Second * 10,
		// AggregationBuckets:         1,
		// Delay:                      time.Second * 10,
	}
	s := NewProvider(cfg, logger, mockEP, mockStorage)
	s.Start()

	for i := 0; i < 100; i++ {
		strategy, _ := s.GetSamplingStrategy(context.Background(), "svcA")
		if len(strategy.OperationSampling.PerOperationStrategies) != 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	s.Close()

	strategy, err := s.GetSamplingStrategy(context.Background(), "svcA")
	require.NoError(t, err)
	require.Len(t, strategy.OperationSampling.PerOperationStrategies, 4)
	strategies := strategy.OperationSampling.PerOperationStrategies

	for _, s := range strategies {
		switch s.Operation {
		case "GET":
			assert.Equal(t, 0.001, s.ProbabilisticSampling.SamplingRate,
				"Already at 1QPS, no probability change")
		case "POST":
			assert.Equal(t, 0.001, s.ProbabilisticSampling.SamplingRate,
				"Within epsilon of 1QPS, no probability change")
		case "PUT":
			assert.InEpsilon(t, 0.002, s.ProbabilisticSampling.SamplingRate, 0.025,
				"Under sampled, double probability")
		case "DELETE":
			assert.InEpsilon(t, 0.0005, s.ProbabilisticSampling.SamplingRate, 0.025,
				"Over sampled, halve probability")
		}
	}
}

func TestPrependBucket(t *testing.T) {
	p := &PostAggregator{AggregatorSettings: AggregatorSettings{AggregationBuckets: 1}}
	p.prependThroughputBucket(&throughputBucket{interval: time.Minute})
	require.Len(t, p.throughputs, 1)
	assert.Equal(t, time.Minute, p.throughputs[0].interval)

	p.prependThroughputBucket(&throughputBucket{interval: 2 * time.Minute})
	require.Len(t, p.throughputs, 1)
	assert.Equal(t, 2*time.Minute, p.throughputs[0].interval)
}

func TestConstructorFailure(t *testing.T) {
	logger := zap.NewNop()

	cfg := AggregatorSettings{
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

func TestGenerateStrategyResponses(t *testing.T) {
	probabilities := model.ServiceOperationProbabilities{
		"svcA": map[string]float64{
			"GET": 0.5,
		},
	}
	p := &Provider{
		probabilities: probabilities,
		ProviderSettings: ProviderSettings{
			InitialSamplingProbability: 0.001,
			MinSamplesPerSecond:        0.0001,
		},
	}
	p.generateStrategyResponses()

	expectedResponse := map[string]*api_v2.SamplingStrategyResponse{
		"svcA": {
			StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC,
			OperationSampling: &api_v2.PerOperationSamplingStrategies{
				DefaultSamplingProbability:       0.001,
				DefaultLowerBoundTracesPerSecond: 0.0001,
				PerOperationStrategies: []*api_v2.OperationSamplingStrategy{
					{
						Operation: "GET",
						ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
							SamplingRate: 0.5,
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expectedResponse, p.strategyResponses)
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
					"GET": {Count: 3, Probabilities: map[string]struct{}{"0.001000": {}}},
					"PUT": {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
				},
				"svcB": map[string]*model.Throughput{
					"PUT": {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
				},
			},
			interval: 60 * time.Second,
		},
	}

	p := &PostAggregator{
		AggregatorSettings: AggregatorSettings{
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
	assert.Equal(t, map[string]float64{"GET": 0.002, "PUT": 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET is no longer reported, we should not increase it's probability since we don't know if it's adaptively sampled
	// until we get at least a lowerbound span or a probability span with the right probability.
	// svcB:PUT is only reporting lowerbound, we should boost it's probability
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"PUT": {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"GET": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				"PUT": {Count: 0, Probabilities: map[string]struct{}{"0.002000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.002, "PUT": 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.004, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET is lower bound sampled, increase its probability
	// svcB:PUT is not reported but we should boost it's probability since the previous calculation showed that
	// it's using adaptive sampling
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"GET": {Count: 0, Probabilities: map[string]struct{}{"0.002000": {}}},
				"PUT": {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"GET": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.004, "PUT": 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.008, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET is finally adaptively probabilistically sampled!
	// svcB:PUT stopped using adaptive sampling
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"GET": {Count: 1, Probabilities: map[string]struct{}{"0.004000": {}}},
				"PUT": {Count: 60, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"GET": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				"PUT": {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.008, "PUT": 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.008, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"PUT": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"GET": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				"PUT": {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.016, "PUT": 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.008, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT starts to use adaptive sampling again
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"PUT": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"GET": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
				"PUT": {Count: 1, Probabilities: map[string]struct{}{"0.008000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.032, "PUT": 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.016, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"PUT": {Count: 30, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"GET": {Count: 15, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.064, "PUT": 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.032, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"PUT": {Count: 20, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"GET": {Count: 10, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.128, "PUT": 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.064, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps

	// svcA:GET didn't report anything
	// svcB:PUT didn't report anything
	p.prependThroughputBucket(&throughputBucket{
		throughput: serviceOperationThroughput{
			"svcA": map[string]*model.Throughput{
				"PUT": {Count: 20, Probabilities: map[string]struct{}{"0.001000": {}}},
				"GET": {Count: 120, Probabilities: map[string]struct{}{"0.128000": {}}},
			},
			"svcB": map[string]*model.Throughput{
				"PUT": {Count: 60, Probabilities: map[string]struct{}{"0.064000": {}}},
				"GET": {Count: 10, Probabilities: map[string]struct{}{"0.001000": {}}},
			},
		},
		interval: 60 * time.Second,
	})

	probabilities, qps = p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.0882586677054928, "PUT": 0.001468867216804201}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"PUT": 0.09587513707888091, "GET": 0.002}, probabilities["svcB"])

	p.probabilities = probabilities
	p.qps = qps
}
