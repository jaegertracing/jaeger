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
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	ss "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

type strategyStore struct {
	// to allow concurrent update of sampling strategies
	lock sync.RWMutex

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

	// Read strategies
	strategies, err := loadStrategies(options.StrategiesFile)
	if err != nil {
		return nil, err
	}
	h.parseStrategies(strategies)

	// Watch strategies file for changes.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("failed to create a new watcher for the sampling strategies file", zap.Error(err))
	}

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					logger.Warn("the sampling strategies file has been removed")

					watcher.Remove(event.Name)
					watcher.Add(options.StrategiesFile)

					s, err := loadStrategies(options.StrategiesFile)
					if err != nil {
						logger.Warn("Error while parsing strategies file. Using the last saved configuration.", zap.Error(err))
					} else {
						logger.Info("Updating sampling strategies file!", zap.Any("Strategies", s))
						h.parseStrategies(s)
					}
					continue
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					s, err := loadStrategies(options.StrategiesFile)
					if err != nil {
						logger.Warn("Error while parsing strategies file. Using the last saved configuration.", zap.Error(err))
					} else {
						logger.Info("Updating sampling strategies file!", zap.Any("Strategies", s))
						h.parseStrategies(s)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Error("event", zap.Error(err))
			}
		}
	}()

	err = watcher.Add(options.StrategiesFile)
	if err != nil {
		logger.Error("error adding watcher to file", zap.String("file", options.StrategiesFile), zap.Error(err))
	} else {
		logger.Info("watching", zap.String("file", options.StrategiesFile))
	}

	return h, nil
}

// GetSamplingStrategy implements StrategyStore#GetSamplingStrategy.
func (h *strategyStore) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	if strategy, ok := h.serviceStrategies[serviceName]; ok {
		return strategy, nil
	}

	return h.defaultStrategy, nil
}

// TODO good candidate for a global util function
func loadStrategies(strategiesFile string) (*strategies, error) {
	if strategiesFile == "" {
		return nil, nil
	}
	bytes, err := ioutil.ReadFile(strategiesFile) /* nolint #nosec , this comes from an admin, not user */
	if err != nil {
		return nil, errors.Wrap(err, "Failed to open strategies file")
	}
	var strategies strategies
	if err := json.Unmarshal(bytes, &strategies); err != nil {
		return nil, errors.Wrap(err, "Failed to unmarshal strategies")
	}
	return &strategies, nil
}

func (h *strategyStore) parseStrategies(strategies *strategies) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.defaultStrategy = defaultStrategyResponse()

	if strategies == nil {
		h.logger.Info("No sampling strategies provided, using defaults")
		return
	}
	if strategies.DefaultStrategy != nil {
		h.defaultStrategy = h.parseServiceStrategies(strategies.DefaultStrategy)
	}

	merge := true
	if h.defaultStrategy.OperationSampling == nil ||
		h.defaultStrategy.OperationSampling.PerOperationStrategies == nil {
		merge = false
	}

	for _, s := range strategies.ServiceStrategies {
		h.serviceStrategies[s.Service] = h.parseServiceStrategies(s)

		// Merge with the default operation strategies, because only merging with
		// the default strategy has no effect on service strategies (the default strategy
		// is not merged with and only used as a fallback).
		opS := h.serviceStrategies[s.Service].OperationSampling
		if opS == nil {
			// Service has no per-operation strategies, so just reference the default settings.
			h.serviceStrategies[s.Service].OperationSampling = h.defaultStrategy.OperationSampling
			continue
		}

		if merge {
			opS.PerOperationStrategies = mergePerOperationSamplingStrategies(
				opS.PerOperationStrategies,
				h.defaultStrategy.OperationSampling.PerOperationStrategies)
		}
	}
}

// mergePerOperationStrategies merges two operation strategies a and b, where a takes precedence over b.
func mergePerOperationSamplingStrategies(
	a, b []*sampling.OperationSamplingStrategy,
) []*sampling.OperationSamplingStrategy {
	m := make(map[string]bool)
	for _, aOp := range a {
		m[aOp.Operation] = true
	}
	for _, bOp := range b {
		if m[bOp.Operation] {
			continue
		}
		a = append(a, bOp)
	}
	return a
}

func (h *strategyStore) parseServiceStrategies(strategy *serviceStrategy) *sampling.SamplingStrategyResponse {
	resp := h.parseStrategy(&strategy.strategy)
	if len(strategy.OperationStrategies) == 0 {
		return resp
	}
	opS := &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability: defaultSamplingProbability,
	}
	if resp.StrategyType == sampling.SamplingStrategyType_PROBABILISTIC {
		opS.DefaultSamplingProbability = resp.ProbabilisticSampling.SamplingRate
	}
	for _, operationStrategy := range strategy.OperationStrategies {
		s, ok := h.parseOperationStrategy(operationStrategy, opS)
		if !ok {
			continue
		}

		opS.PerOperationStrategies = append(opS.PerOperationStrategies,
			&sampling.OperationSamplingStrategy{
				Operation:             operationStrategy.Operation,
				ProbabilisticSampling: s.ProbabilisticSampling,
			})
	}
	resp.OperationSampling = opS
	return resp
}

func (h *strategyStore) parseOperationStrategy(
	strategy *operationStrategy,
	parent *sampling.PerOperationSamplingStrategies,
) (s *sampling.SamplingStrategyResponse, ok bool) {
	s = h.parseStrategy(&strategy.strategy)
	if s.StrategyType == sampling.SamplingStrategyType_RATE_LIMITING {
		// TODO OperationSamplingStrategy only supports probabilistic sampling
		h.logger.Warn(
			fmt.Sprintf(
				"Operation strategies only supports probabilistic sampling at the moment,"+
					"'%s' defaulting to probabilistic sampling with probability %f",
				strategy.Operation, parent.DefaultSamplingProbability),
			zap.Any("strategy", strategy))
		return nil, false
	}
	return s, true
}

func (h *strategyStore) parseStrategy(strategy *strategy) *sampling.SamplingStrategyResponse {
	switch strategy.Type {
	case samplerTypeProbabilistic:
		return &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
				SamplingRate: strategy.Param,
			},
		}
	case samplerTypeRateLimiting:
		return &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
				MaxTracesPerSecond: int16(strategy.Param),
			},
		}
	default:
		h.logger.Warn("Failed to parse sampling strategy", zap.Any("strategy", strategy))
		return defaultStrategyResponse()
	}
}

func deepCopy(s *sampling.SamplingStrategyResponse) *sampling.SamplingStrategyResponse {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	enc.Encode(*s)
	var copy sampling.SamplingStrategyResponse
	dec.Decode(&copy)
	return &copy
}
