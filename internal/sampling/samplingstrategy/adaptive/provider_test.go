// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	epmocks "github.com/jaegertracing/jaeger/internal/leaderelection/mocks"
	smocks "github.com/jaegertracing/jaeger/internal/storage/v1/samplingstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/samplingstore/model"
)

func TestProviderLoadProbabilities(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)

	p := &Provider{storage: mockStorage}
	require.Nil(t, p.probabilities)
	p.loadProbabilities()
	require.NotNil(t, p.probabilities)
}

func TestProviderRunUpdateProbabilitiesLoop(t *testing.T) {
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

func TestProviderRealisticRunCalculationLoop(t *testing.T) {
	t.Skip("Skipped realistic calculation loop test")
	logger := zap.NewNop()
	// NB: This is an extremely long test since it uses near realistic (1/6th scale) processor config values
	testThroughputs := []*model.Throughput{
		{Service: "svcA", Operation: http.MethodGet, Count: 10},
		{Service: "svcA", Operation: http.MethodPost, Count: 9},
		{Service: "svcA", Operation: http.MethodPut, Count: 5},
		{Service: "svcA", Operation: http.MethodDelete, Count: 20},
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
	cfg := Options{
		TargetSamplesPerSecond:     1.0,
		DeltaTolerance:             0.2,
		InitialSamplingProbability: 0.001,
		CalculationInterval:        time.Second * 10,
		AggregationBuckets:         1,
		Delay:                      time.Second * 10,
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
		case http.MethodGet:
			assert.InDelta(t, 0.001, s.ProbabilisticSampling.SamplingRate, 1e-4,
				"Already at 1QPS, no probability change")
		case http.MethodPost:
			assert.InDelta(t, 0.001, s.ProbabilisticSampling.SamplingRate, 1e-4,
				"Within epsilon of 1QPS, no probability change")
		case http.MethodPut:
			assert.InEpsilon(t, 0.002, s.ProbabilisticSampling.SamplingRate, 0.025,
				"Under sampled, double probability")
		case http.MethodDelete:
			assert.InEpsilon(t, 0.0005, s.ProbabilisticSampling.SamplingRate, 0.025,
				"Over sampled, halve probability")
		}
	}
}

func TestProviderGenerateStrategyResponses(t *testing.T) {
	probabilities := model.ServiceOperationProbabilities{
		"svcA": map[string]float64{
			http.MethodGet: 0.5,
		},
	}
	p := &Provider{
		probabilities: probabilities,
		Options: Options{
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
						Operation: http.MethodGet,
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
