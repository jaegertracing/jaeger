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
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	ss "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// null represents "null" JSON value and
// it un-marshals to nil pointer.
var nullJSON = []byte("null")

type strategyStore struct {
	logger *zap.Logger

	storedStrategies atomic.Value // holds *storedStrategies

	ctx        context.Context
	cancelFunc context.CancelFunc
}

type storedStrategies struct {
	defaultStrategy   *sampling.SamplingStrategyResponse
	serviceStrategies map[string]*sampling.SamplingStrategyResponse
}

type strategyLoader func() ([]byte, error)

// NewStrategyStore creates a strategy store that holds static sampling strategies.
func NewStrategyStore(options Options, logger *zap.Logger) (ss.StrategyStore, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	h := &strategyStore{
		logger:     logger,
		ctx:        ctx,
		cancelFunc: cancelFunc,
	}
	h.storedStrategies.Store(defaultStrategies())

	if options.StrategiesFile == "" {
		h.parseStrategies(nil)
		return h, nil
	}

	loadFn := samplingStrategyLoader(options.StrategiesFile)
	strategies, err := loadStrategies(loadFn)
	if err != nil {
		return nil, err
	}
	h.parseStrategies(strategies)

	if options.ReloadInterval > 0 {
		go h.autoUpdateStrategies(options.ReloadInterval, loadFn)
	}
	return h, nil
}

// GetSamplingStrategy implements StrategyStore#GetSamplingStrategy.
func (h *strategyStore) GetSamplingStrategy(_ context.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	ss := h.storedStrategies.Load().(*storedStrategies)
	serviceStrategies := ss.serviceStrategies
	if strategy, ok := serviceStrategies[serviceName]; ok {
		return strategy, nil
	}
	h.logger.Debug("sampling strategy not found, using default", zap.String("service", serviceName))
	return ss.defaultStrategy, nil
}

// Close stops updating the strategies
func (h *strategyStore) Close() {
	h.cancelFunc()
}

func downloadSamplingStrategies(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download sampling strategies: %w", err)
	}

	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err = buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read sampling strategies HTTP response body: %w", err)
	}

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nullJSON, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"receiving %s while downloading strategies file: %s",
			resp.Status,
			buf.String(),
		)
	}

	return buf.Bytes(), nil
}

func isURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func samplingStrategyLoader(strategiesFile string) strategyLoader {
	if isURL(strategiesFile) {
		return func() ([]byte, error) {
			return downloadSamplingStrategies(strategiesFile)
		}
	}

	return func() ([]byte, error) {
		currBytes, err := ioutil.ReadFile(filepath.Clean(strategiesFile))
		if err != nil {
			return nil, fmt.Errorf("failed to read strategies file %s: %w", strategiesFile, err)
		}
		return currBytes, nil
	}
}

func (h *strategyStore) autoUpdateStrategies(interval time.Duration, loader strategyLoader) {
	lastValue := string(nullJSON)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			lastValue = h.reloadSamplingStrategy(loader, lastValue)
		case <-h.ctx.Done():
			return
		}
	}
}

func (h *strategyStore) reloadSamplingStrategy(loadFn strategyLoader, lastValue string) string {
	newValue, err := loadFn()
	if err != nil {
		h.logger.Error("failed to re-load sampling strategies", zap.Error(err))
		return lastValue
	}
	if lastValue == string(newValue) {
		return lastValue
	}
	if err := h.updateSamplingStrategy(newValue); err != nil {
		h.logger.Error("failed to update sampling strategies", zap.Error(err))
		return lastValue
	}
	return string(newValue)
}

func (h *strategyStore) updateSamplingStrategy(bytes []byte) error {
	var strategies strategies
	if err := json.Unmarshal(bytes, &strategies); err != nil {
		return fmt.Errorf("failed to unmarshal sampling strategies: %w", err)
	}
	h.parseStrategies(&strategies)
	h.logger.Info("Updated sampling strategies:" + string(bytes))
	return nil
}

// TODO good candidate for a global util function
func loadStrategies(loadFn strategyLoader) (*strategies, error) {
	strategyBytes, err := loadFn()
	if err != nil {
		return nil, err
	}

	var strategies *strategies
	if err := json.Unmarshal(strategyBytes, &strategies); err != nil {
		return nil, fmt.Errorf("failed to unmarshal strategies: %w", err)
	}
	return strategies, nil
}

func (h *strategyStore) parseStrategies(strategies *strategies) {
	if strategies == nil {
		h.logger.Info("No sampling strategies provided or URL is unavailable, using defaults")
		return
	}
	newStore := defaultStrategies()
	if strategies.DefaultStrategy != nil {
		newStore.defaultStrategy = h.parseServiceStrategies(strategies.DefaultStrategy)
	}

	merge := true
	if newStore.defaultStrategy.OperationSampling == nil ||
		newStore.defaultStrategy.OperationSampling.PerOperationStrategies == nil {
		merge = false
	}

	for _, s := range strategies.ServiceStrategies {
		newStore.serviceStrategies[s.Service] = h.parseServiceStrategies(s)

		// Merge with the default operation strategies, because only merging with
		// the default strategy has no effect on service strategies (the default strategy
		// is not merged with and only used as a fallback).
		opS := newStore.serviceStrategies[s.Service].OperationSampling
		if opS == nil {
			if newStore.defaultStrategy.OperationSampling == nil ||
				newStore.serviceStrategies[s.Service].ProbabilisticSampling == nil {
				continue
			}
			// Service has no per-operation strategies, so just reference the default settings and change default samplingRate.
			newOpS := *newStore.defaultStrategy.OperationSampling
			newOpS.DefaultSamplingProbability = newStore.serviceStrategies[s.Service].ProbabilisticSampling.SamplingRate
			newStore.serviceStrategies[s.Service].OperationSampling = &newOpS
			continue
		}
		if merge {
			opS.PerOperationStrategies = mergePerOperationSamplingStrategies(
				opS.PerOperationStrategies,
				newStore.defaultStrategy.OperationSampling.PerOperationStrategies)
		}
	}
	h.storedStrategies.Store(newStore)
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
