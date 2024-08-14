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
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

const defaultFollowerProbabilityInterval = 20 * time.Second

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
func (ss *Provider) Start() error {
	ss.logger.Info("starting adaptive sampling service")
	ss.loadProbabilities()
	ss.generateStrategyResponses()

	ss.bgFinished.Add(1)
	go func() {
		ss.runUpdateProbabilitiesLoop()
		ss.bgFinished.Done()
	}()

	return nil
}

// Close stops the service from loading probabilities and generating strategies.
func (ss *Provider) Close() error {
	ss.logger.Info("stopping adaptive sampling service")
	close(ss.shutdown)
	ss.bgFinished.Wait()
	return nil
}
