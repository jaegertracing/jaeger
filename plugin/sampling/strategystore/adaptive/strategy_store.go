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
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

const defaultFollowerProbabilityInterval = 20 * time.Second

type StrategyStore struct {
	sync.RWMutex
	Options

	electionParticipant leaderelection.ElectionParticipant
	storage             samplingstore.Store
	logger              *zap.Logger

	// probabilities contains the latest calculated sampling probabilities for service operations.
	probabilities model.ServiceOperationProbabilities

	// strategyResponses is the cache of the sampling strategies for every service, in Thrift format.
	// TODO change this to work with protobuf model instead, to support gRPC endpoint.
	strategyResponses map[string]*api_v2.SamplingStrategyResponse

	// followerRefreshInterval determines how often the follower processor updates its probabilities.
	// Given only the leader writes probabilities, the followers need to fetch the probabilities into
	// cache.
	followerRefreshInterval time.Duration

	shutdown   chan struct{}
	bgFinished sync.WaitGroup
}

// NewStrategyStore creates a strategy store that holds adaptive sampling strategies.
func NewStrategyStore(options Options, logger *zap.Logger, participant leaderelection.ElectionParticipant, store samplingstore.Store) (*StrategyStore, error) {
	return &StrategyStore{
		Options:                 options,
		storage:                 store,
		probabilities:           make(model.ServiceOperationProbabilities),
		strategyResponses:       make(map[string]*api_v2.SamplingStrategyResponse),
		logger:                  logger,
		electionParticipant:     participant,
		followerRefreshInterval: defaultFollowerProbabilityInterval,
	}, nil
}

// Start initializes and starts the sampling processor which regularly calculates sampling probabilities.
func (ss *StrategyStore) Start() error {
	ss.logger.Info("starting adaptive sampling processor")
	if err := ss.electionParticipant.Start(); err != nil {
		return err
	}
	ss.shutdown = make(chan struct{})
	ss.loadProbabilities()
	ss.generateStrategyResponses()
	ss.runBackground(ss.runUpdateProbabilitiesLoop)
	return nil
}

// GetSamplingStrategy implements Thrift endpoint for retrieving sampling strategy for a service.
func (ss *StrategyStore) GetSamplingStrategy(_ context.Context, service string) (*api_v2.SamplingStrategyResponse, error) {
	ss.RLock()
	defer ss.RUnlock()
	if strategy, ok := ss.strategyResponses[service]; ok {
		return strategy, nil
	}
	return ss.generateDefaultSamplingStrategyResponse(), nil
}

func (ss *StrategyStore) loadProbabilities() {
	// TODO GetLatestProbabilities API can be changed to return the latest measured qps for initialization
	probabilities, err := ss.storage.GetLatestProbabilities()
	if err != nil {
		ss.logger.Warn("failed to initialize probabilities", zap.Error(err))
		return
	}
	ss.Lock()
	defer ss.Unlock()
	ss.probabilities = probabilities
}

// runUpdateProbabilitiesLoop is a loop that reads probabilities from storage.
// The follower updates its local cache with the latest probabilities and serves them.
func (ss *StrategyStore) runUpdateProbabilitiesLoop() {
	select {
	case <-time.After(addJitter(ss.followerRefreshInterval)):
	case <-ss.shutdown:
		return
	}

	ticker := time.NewTicker(ss.followerRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Only load probabilities if this processor doesn't hold the leader lock
			if !ss.isLeader() {
				ss.loadProbabilities()
				ss.generateStrategyResponses()
			}
		case <-ss.shutdown:
			return
		}
	}
}

func (ss *StrategyStore) isLeader() bool {
	return ss.electionParticipant.IsLeader()
}

// generateStrategyResponses generates and caches SamplingStrategyResponse from the calculated sampling probabilities.
func (ss *StrategyStore) generateStrategyResponses() {
	ss.RLock()
	strategies := make(map[string]*api_v2.SamplingStrategyResponse)
	for svc, opProbabilities := range ss.probabilities {
		opStrategies := make([]*api_v2.OperationSamplingStrategy, len(opProbabilities))
		var idx int
		for op, probability := range opProbabilities {
			opStrategies[idx] = &api_v2.OperationSamplingStrategy{
				Operation: op,
				ProbabilisticSampling: &api_v2.ProbabilisticSamplingStrategy{
					SamplingRate: probability,
				},
			}
			idx++
		}
		strategy := ss.generateDefaultSamplingStrategyResponse()
		strategy.OperationSampling.PerOperationStrategies = opStrategies
		strategies[svc] = strategy
	}
	ss.RUnlock()

	ss.Lock()
	defer ss.Unlock()
	ss.strategyResponses = strategies
}

func (ss *StrategyStore) generateDefaultSamplingStrategyResponse() *api_v2.SamplingStrategyResponse {
	return &api_v2.SamplingStrategyResponse{
		StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC,
		OperationSampling: &api_v2.PerOperationSamplingStrategies{
			DefaultSamplingProbability:       ss.InitialSamplingProbability,
			DefaultLowerBoundTracesPerSecond: ss.MinSamplesPerSecond,
		},
	}
}

func (ss *StrategyStore) runBackground(f func()) {
	ss.bgFinished.Add(1)
	go func() {
		f()
		ss.bgFinished.Done()
	}()
}

// Close stops the processor from calculating probabilities.
func (ss *StrategyStore) Close() error {
	ss.logger.Info("stopping adaptive sampling processor")
	err := ss.electionParticipant.Close()
	if ss.shutdown != nil {
		close(ss.shutdown)
	}
	ss.bgFinished.Wait()
	return err
}
