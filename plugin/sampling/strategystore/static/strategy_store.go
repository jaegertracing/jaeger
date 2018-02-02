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

package static

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	ss "github.com/jaegertracing/jaeger/pkg/sampling/strategystore"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type strategyStore struct {
	logger *zap.Logger

	defaultStrategy   *sampling.SamplingStrategyResponse
	serviceStrategies map[string]*sampling.SamplingStrategyResponse
}

// NewStrategyStore creates a strategy store that holds static sampling strategies.
func NewStrategyStore(options Options, logger *zap.Logger) (ss.StrategyStore, error) {
	h := &strategyStore{
		logger:            logger,
		serviceStrategies: make(map[string]*sampling.SamplingStrategyResponse),
	}
	strategies, err := loadStrategies(options.StrategiesFile)
	if err != nil {
		return nil, err
	}
	h.parseStrategies(strategies)
	return h, nil
}

// GetSamplingStrategy implements StrategyStore#GetSamplingStrategy.
func (h *strategyStore) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	if strategy, ok := h.serviceStrategies[serviceName]; ok {
		return strategy, nil
	}
	return h.defaultStrategy, nil
}

// TODO good candidate for a global util function
func loadStrategies(strategiesFile string) (*ss.Strategies, error) {
	if strategiesFile == "" {
		return nil, nil
	}
	bytes, err := ioutil.ReadFile(strategiesFile)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to open strategies file")
	}
	var strategies ss.Strategies
	if err := json.Unmarshal(bytes, &strategies); err != nil {
		return nil, errors.Wrap(err, "Failed to unmarshal strategies")
	}
	return &strategies, nil
}

func (h *strategyStore) parseStrategies(strategies *ss.Strategies) {
	h.defaultStrategy = &ss.DefaultStrategy
	if strategies == nil {
		h.logger.Info("No sampling strategies provided, using defaults")
		return
	}
	if strategies.DefaultStrategy != nil {
		h.defaultStrategy = h.parseStrategy(strategies.DefaultStrategy)
	}
	for _, s := range strategies.ServiceStrategies {
		h.serviceStrategies[s.Service] = h.parseStrategy(&s.Strategy)
	}
}

func (h *strategyStore) parseStrategy(strategy *ss.Strategy) *sampling.SamplingStrategyResponse {
	switch strategy.Type {
	case ss.SamplerTypeProbabilistic:
		return &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
				SamplingRate: strategy.Param,
			},
		}
	case ss.SamplerTypeRateLimiting:
		return &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
				MaxTracesPerSecond: int16(strategy.Param),
			},
		}
	default:
		h.logger.Warn("Failed to parse sampling strategy", zap.Any("strategy", strategy))
		return &ss.DefaultStrategy
	}
}
