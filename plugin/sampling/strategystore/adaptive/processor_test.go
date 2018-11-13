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
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	jio "github.com/jaegertracing/jaeger/pkg/io"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/sampling/internal/calculationstrategy"
	epmocks "github.com/jaegertracing/jaeger/plugin/sampling/internal/leaderelection/mocks"
	smocks "github.com/jaegertracing/jaeger/storage/samplingstore/mocks"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

var (
	testThroughputs = []*model.Throughput{
		{Service: "svcA", Operation: "GET", Count: 4, Probabilities: map[string]struct{}{"0.1": {}}},
		{Service: "svcA", Operation: "GET", Count: 4, Probabilities: map[string]struct{}{"0.2": {}}},
		{Service: "svcA", Operation: "PUT", Count: 5, Probabilities: map[string]struct{}{"0.1": {}}},
		{Service: "svcB", Operation: "GET", Count: 3, Probabilities: map[string]struct{}{"0.1": {}}},
	}

	testThroughputBuckets = []*throughputBucket{
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

	errTestStorage = errors.New("Storage error")

	testCalculator = calculationstrategy.Calculate(func(targetQPS, qps, oldProbability float64) float64 {
		factor := targetQPS / qps
		return oldProbability * factor
	})
)

func TestAggregateThroughput(t *testing.T) {
	p := &processor{}
	aggregatedThroughput := p.aggregateThroughput(testThroughputs)
	require.Len(t, aggregatedThroughput, 2)

	throughput, ok := aggregatedThroughput["svcA"]
	require.True(t, ok)
	require.Len(t, throughput, 2)

	opThroughput, ok := throughput["GET"]
	require.True(t, ok)
	assert.Equal(t, opThroughput.Count, int64(8))
	assert.Equal(t, opThroughput.Probabilities, map[string]struct{}{"0.1": {}, "0.2": {}})

	opThroughput, ok = throughput["PUT"]
	require.True(t, ok)
	assert.Equal(t, opThroughput.Count, int64(5))
	assert.Equal(t, opThroughput.Probabilities, map[string]struct{}{"0.1": {}})

	throughput, ok = aggregatedThroughput["svcB"]
	require.True(t, ok)
	require.Len(t, throughput, 1)

	opThroughput, ok = throughput["GET"]
	require.True(t, ok)
	assert.Equal(t, opThroughput.Count, int64(3))
	assert.Equal(t, opThroughput.Probabilities, map[string]struct{}{"0.1": {}})
}

func TestInitializeThroughput(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*19), time.Time{}.Add(time.Minute*20)).
		Return(testThroughputs, nil)
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*18), time.Time{}.Add(time.Minute*19)).
		Return([]*model.Throughput{{Service: "svcA", Operation: "GET", Count: 7}}, nil)
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*17), time.Time{}.Add(time.Minute*18)).
		Return([]*model.Throughput{}, nil)
	p := &processor{storage: mockStorage, buckets: 3, Options: Options{CalculationInterval: time.Minute}}
	p.initializeThroughput(time.Time{}.Add(time.Minute * 20))

	require.Len(t, p.throughputs, 2)
	require.Len(t, p.throughputs[0].throughput, 2)
	assert.Equal(t, p.throughputs[0].interval, time.Minute)
	assert.Equal(t, p.throughputs[0].endTime, time.Time{}.Add(time.Minute*20))
	require.Len(t, p.throughputs[1].throughput, 1)
	assert.Equal(t, p.throughputs[1].interval, time.Minute)
	assert.Equal(t, p.throughputs[1].endTime, time.Time{}.Add(time.Minute*19))
}

func TestInitializeThroughputFailure(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", time.Time{}.Add(time.Minute*19), time.Time{}.Add(time.Minute*20)).
		Return(nil, errTestStorage)
	p := &processor{storage: mockStorage, buckets: 1, Options: Options{CalculationInterval: time.Minute}}
	p.initializeThroughput(time.Time{}.Add(time.Minute * 20))

	assert.Len(t, p.throughputs, 0)
}

func TestCalculateQPS(t *testing.T) {
	qps := calculateQPS(int64(90), 60*time.Second)
	assert.Equal(t, 1.5, qps)

	qps = calculateQPS(int64(45), 60*time.Second)
	assert.Equal(t, 0.75, qps)
}

func TestGenerateOperationQPS(t *testing.T) {
	p := &processor{throughputs: testThroughputBuckets, buckets: 10, Options: Options{LookbackQPSCount: 10}}
	svcOpQPS := p.generateOperationQPS()
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
	svcOpQPS = p.generateOperationQPS()
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
	p := &processor{throughputs: testThroughputBuckets, buckets: 10, Options: Options{LookbackQPSCount: 1}}
	svcOpQPS := p.generateOperationQPS()
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

	svcOpQPS = p.generateOperationQPS()
	require.Len(t, svcOpQPS, 2)

	opQPS, ok = svcOpQPS["svcA"]
	require.True(t, ok)
	require.Len(t, opQPS, 2)

	assert.Equal(t, []float64{0.5}, opQPS["GET"])
	assert.Equal(t, []float64{1.0}, opQPS["PUT"])
}

func TestCalculateWeightedQPS(t *testing.T) {
	p := processor{weightsCache: newWeightsCache()}
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
	cfg := Options{
		TargetQPS:                  1.0,
		QPSEquivalenceThreshold:    0.2,
		DefaultSamplingProbability: 0.001,
		MinSamplingProbability:     0.00001,
	}
	p := &processor{
		Options:               cfg,
		probabilities:         probabilities,
		probabilityCalculator: testCalculator,
		throughputs:           throughputs,
		serviceCache:          []samplingCache{{"svcA": {}, "svcB": {}}},
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
	metrics := metrics.NewLocalFactory(0)
	p := &processor{
		Options: Options{
			TargetQPS:                  1.0,
			QPSEquivalenceThreshold:    0.2,
			DefaultSamplingProbability: 0.001,
			LookbackQPSCount:           10,
		},
		throughputs: testThroughputBuckets, probabilities: prevProbabilities, qps: qps,
		weightsCache: newWeightsCache(), probabilityCalculator: testCalculator,
		operationsCalculatedGauge: metrics.Gauge("test", nil),
	}
	probabilities, qps := p.calculateProbabilitiesAndQPS()

	require.Len(t, probabilities, 2)
	assert.Equal(t, map[string]float64{"GET": 0.00136, "PUT": 0.001}, probabilities["svcA"])
	assert.Equal(t, map[string]float64{"GET": 0.16, "PUT": 0.03}, probabilities["svcB"])

	require.Len(t, qps, 2)
	assert.Equal(t, map[string]float64{"GET": 0.7352941176470588, "PUT": 1}, qps["svcA"])
	assert.Equal(t, map[string]float64{"GET": 0.5147058823529411, "PUT": 0.25}, qps["svcB"])

	_, gauges := metrics.LocalBackend.Snapshot()
	assert.EqualValues(t, 4, gauges["test"])
}

func TestRunCalculationLoop(t *testing.T) {
	logger := zap.NewNop()
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return(testThroughputs, nil)
	mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{}, errTestStorage)
	mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.AnythingOfType("model.ServiceOperationProbabilities"),
		mock.AnythingOfType("model.ServiceOperationQPS")).Return(errTestStorage)
	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("Start").Return()
	mockEP.On("IsLeader").Return(true, nil)

	cfg := Options{
		TargetQPS:                    1.0,
		QPSEquivalenceThreshold:      0.1,
		DefaultSamplingProbability:   0.001,
		CalculationInterval:          time.Millisecond * 5,
		LookbackInterval:             time.Millisecond * 10,
		Delay:                        time.Millisecond * 5,
		LeaderLeaseRefreshInterval:   time.Millisecond,
		FollowerLeaseRefreshInterval: time.Second,
		LookbackQPSCount:             10,
	}
	p, err := NewProcessor(cfg, "host", mockStorage, mockEP, metrics.NullFactory, logger)
	require.NoError(t, err)
	p.(jio.Starter).Start()

	for i := 0; i < 1000; i++ {
		strategy, _ := p.GetSamplingStrategy("svcA")
		if len(strategy.OperationSampling.PerOperationStrategies) != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	p.(io.Closer).Close()

	strategy, err := p.GetSamplingStrategy("svcA")
	assert.NoError(t, err)
	assert.Len(t, strategy.OperationSampling.PerOperationStrategies, 2)
}

func TestRunCalculationLoop_GetThroughputError(t *testing.T) {
	logger, logBuffer := testutils.NewLogger()
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return(nil, errTestStorage)
	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("IsLeader").Return(false, nil)

	cfg := Options{
		CalculationInterval: time.Millisecond * 5,
		LookbackInterval:    time.Millisecond * 10,
		LookbackQPSCount:    10,
	}
	proc, err := NewProcessor(cfg, "host", mockStorage, mockEP, metrics.NullFactory, logger)
	require.NoError(t, err)
	p := proc.(*processor)
	p.calculationStop = make(chan struct{})
	defer close(p.calculationStop)
	go p.runCalculationLoop()

	for i := 0; i < 1000; i++ {
		// match logs specific to getThroughputErrMsg. We expect to see more than 2, once during
		// initialization and one or more times during the loop.
		if testutils.LogMatcher(2, getThroughputErrMsg, logBuffer.Lines()) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	assert.True(t, testutils.LogMatcher(2, getThroughputErrMsg, logBuffer.Lines()))
}

func TestLoadProbabilities(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)

	p := &processor{storage: mockStorage}
	require.Nil(t, p.probabilities)
	p.loadProbabilities()
	require.NotNil(t, p.probabilities)
}

func TestRunUpdateProbabilitiesLoop(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)
	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("IsLeader").Return(false, nil)

	p := &processor{
		storage:                     mockStorage,
		updateProbabilitiesStop:     make(chan struct{}),
		followerProbabilityInterval: time.Millisecond,
		electionParticipant:         mockEP,
	}
	defer close(p.updateProbabilitiesStop)
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
}

func TestRealisticRunCalculationLoop(t *testing.T) {
	t.Skip("Skipped realistic calculation loop test")
	logger := zap.NewNop()
	// NB: This is an extremely long test since it uses near realistic (1/6th scale) processor config values
	testThroughputs = []*model.Throughput{
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
	mockEP.On("IsLeader").Return(true, nil)
	cfg := Options{
		TargetQPS:                  1.0,
		QPSEquivalenceThreshold:    0.2,
		DefaultSamplingProbability: 0.001,
		CalculationInterval:        time.Second * 10,
		LookbackInterval:           time.Second * 10,
		Delay:                      time.Second * 10,
	}
	p, err := NewProcessor(cfg, "host", mockStorage, mockEP, metrics.NullFactory, logger)
	require.NoError(t, err)
	p.(jio.Starter).Start()

	for i := 0; i < 100; i++ {
		strategy, _ := p.GetSamplingStrategy("svcA")
		if len(strategy.OperationSampling.PerOperationStrategies) != 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	p.(io.Closer).Close()

	strategy, err := p.GetSamplingStrategy("svcA")
	assert.NoError(t, err)
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
	p := &processor{buckets: 1}
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
		TargetQPS:                  1.0,
		QPSEquivalenceThreshold:    0.2,
		DefaultSamplingProbability: 0.001,
		CalculationInterval:        time.Second * 5,
		LookbackInterval:           time.Second,
	}
	_, err := NewProcessor(cfg, "host", nil, nil, metrics.NullFactory, logger)
	assert.EqualError(t, err, "calculationInterval must be less than LookbackInterval")

	cfg.CalculationInterval = 0
	_, err = NewProcessor(cfg, "host", nil, nil, metrics.NullFactory, logger)
	assert.EqualError(t, err, "calculationInterval and LookbackInterval must be greater than 0")

	cfg.CalculationInterval = time.Millisecond
	cfg.LookbackQPSCount = -1
	_, err = NewProcessor(cfg, "host", nil, nil, metrics.NullFactory, logger)
	assert.EqualError(t, err, "lookbackQPSCount cannot be less than 1")
}

func TestGenerateStrategyResponses(t *testing.T) {
	probabilities := model.ServiceOperationProbabilities{
		"svcA": map[string]float64{
			"GET": 0.5,
		},
	}
	p := &processor{probabilities: probabilities, Options: Options{DefaultSamplingProbability: 0.001, LowerBoundTracesPerSecond: 0.0001}}
	p.generateStrategyResponses()

	expectedResponse := map[string]*sampling.SamplingStrategyResponse{
		"svcA": {
			StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
			OperationSampling: &sampling.PerOperationSamplingStrategies{
				DefaultSamplingProbability:       0.001,
				DefaultLowerBoundTracesPerSecond: 0.0001,
				PerOperationStrategies: []*sampling.OperationSamplingStrategy{
					{
						Operation: "GET",
						ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
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
	p := &processor{}
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
		assert.Equal(t, test.expected, p.usingAdaptiveSampling(test.probability, test.service, test.operation, throughput))
	}
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

	p := &processor{
		Options: Options{
			TargetQPS:                  1.0,
			QPSEquivalenceThreshold:    0.002,
			DefaultSamplingProbability: 0.001,
			LookbackQPSCount:           5,
		},
		throughputs: buckets, probabilities: make(model.ServiceOperationProbabilities), buckets: 10,
		qps: make(model.ServiceOperationQPS), weightsCache: newWeightsCache(),
		probabilityCalculator:     calculationstrategy.NewPercentageIncreaseCappedCalculator(1.0),
		serviceCache:              []samplingCache{},
		operationsCalculatedGauge: metrics.NullFactory.Gauge("", nil),
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
