// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/samplingstore/model"
)

const defaultFollowerProbabilityInterval = 20 * time.Second

// Provider is responsible for providing sampling strategies for services.
// It periodically loads sampling probabilities from storage and converts them
// into sampling strategies that are cached and served to clients.
// Provider relies on sampling probabilities being periodically updated by the
// aggregator & post-aggregator.
type Provider struct {
	sync.RWMutex
	Options

	electionParticipant leaderelection.ElectionParticipant
	storage             samplingstore.Store
	logger              *zap.Logger

	// probabilities contains the latest calculated sampling probabilities for service operations.
	probabilities model.ServiceOperationProbabilities

	// strategyResponses is the cache of the sampling strategies for every service, in protobuf format.
	strategyResponses map[string]*api_v2.SamplingStrategyResponse

	// followerRefreshInterval determines how often the follower processor updates its probabilities.
	// Given only the leader writes probabilities, the followers need to fetch the probabilities into
	// cache.
	followerRefreshInterval time.Duration

	shutdown   chan struct{}
	bgFinished sync.WaitGroup
}

// NewProvider creates a strategy store that holds adaptive sampling strategies.
func NewProvider(options Options, logger *zap.Logger, participant leaderelection.ElectionParticipant, store samplingstore.Store) *Provider {
	return &Provider{
		Options:                 options,
		storage:                 store,
		probabilities:           make(model.ServiceOperationProbabilities),
		strategyResponses:       make(map[string]*api_v2.SamplingStrategyResponse),
		logger:                  logger,
		electionParticipant:     participant,
		followerRefreshInterval: defaultFollowerProbabilityInterval,
		shutdown:                make(chan struct{}),
	}
}

// Start initializes and starts the sampling service which regularly loads sampling probabilities and generates strategies.
func (p *Provider) Start() error {
	p.logger.Info("starting adaptive sampling service")
	p.loadProbabilities()
	p.generateStrategyResponses()

	p.bgFinished.Add(1)
	go func() {
		p.runUpdateProbabilitiesLoop()
		p.bgFinished.Done()
	}()

	return nil
}

func (p *Provider) loadProbabilities() {
	// TODO GetLatestProbabilities API can be changed to return the latest measured qps for initialization
	probabilities, err := p.storage.GetLatestProbabilities()
	if err != nil {
		p.logger.Warn("failed to initialize probabilities", zap.Error(err))
		return
	}
	p.Lock()
	defer p.Unlock()
	p.probabilities = probabilities
}

// runUpdateProbabilitiesLoop is a loop that reads probabilities from storage.
// The follower updates its local cache with the latest probabilities and serves them.
func (p *Provider) runUpdateProbabilitiesLoop() {
	select {
	case <-time.After(addJitter(p.followerRefreshInterval)):
		// continue after jitter delay
	case <-p.shutdown:
		return
	}

	ticker := time.NewTicker(p.followerRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Only load probabilities if this strategy_store doesn't hold the leader lock
			if !p.isLeader() {
				p.loadProbabilities()
				p.generateStrategyResponses()
			}
		case <-p.shutdown:
			return
		}
	}
}

func (p *Provider) isLeader() bool {
	return p.electionParticipant.IsLeader()
}

// generateStrategyResponses generates and caches SamplingStrategyResponse from the calculated sampling probabilities.
func (p *Provider) generateStrategyResponses() {
	p.RLock()
	strategies := make(map[string]*api_v2.SamplingStrategyResponse)
	for svc, opProbabilities := range p.probabilities {
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
		strategy := p.generateDefaultSamplingStrategyResponse()
		strategy.OperationSampling.PerOperationStrategies = opStrategies
		strategies[svc] = strategy
	}
	p.RUnlock()

	p.Lock()
	defer p.Unlock()
	p.strategyResponses = strategies
}

func (p *Provider) generateDefaultSamplingStrategyResponse() *api_v2.SamplingStrategyResponse {
	return &api_v2.SamplingStrategyResponse{
		StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC,
		OperationSampling: &api_v2.PerOperationSamplingStrategies{
			DefaultSamplingProbability:       p.InitialSamplingProbability,
			DefaultLowerBoundTracesPerSecond: p.MinSamplesPerSecond,
		},
	}
}

// GetSamplingStrategy implements protobuf endpoint for retrieving sampling strategy for a service.
func (p *Provider) GetSamplingStrategy(_ context.Context, service string) (*api_v2.SamplingStrategyResponse, error) {
	p.RLock()
	defer p.RUnlock()
	if strategy, ok := p.strategyResponses[service]; ok {
		return strategy, nil
	}
	return p.generateDefaultSamplingStrategyResponse(), nil
}

// Close stops the service from loading probabilities and generating strategies.
func (p *Provider) Close() error {
	p.logger.Info("stopping adaptive sampling service")
	close(p.shutdown)
	p.bgFinished.Wait()
	return nil
}
