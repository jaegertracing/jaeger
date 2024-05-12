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
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	span_model "github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/hostname"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

const (
	maxProbabilities = 10
)

type aggregator struct {
	sync.Mutex

	operationsCounter   metrics.Counter
	servicesCounter     metrics.Counter
	currentThroughput   serviceOperationThroughput
	postAggregator      *PostAggregator
	aggregationInterval time.Duration
	storage             samplingstore.Store
	stop                chan struct{}
	bgFinished          sync.WaitGroup
}

// NewAggregator creates a throughput aggregator that simply emits metrics
// about the number of operations seen over the aggregationInterval.
func NewAggregator(options Options, logger *zap.Logger, metricsFactory metrics.Factory, participant leaderelection.ElectionParticipant, store samplingstore.Store) (strategystore.Aggregator, error) {
	hostname, err := hostname.AsIdentifier()
	if err != nil {
		return nil, err
	}
	logger.Info("Using unique participantName in adaptive sampling", zap.String("participantName", hostname))

	postAgg, err := newPostAggregator(options, hostname, store, participant, metricsFactory, logger)
	if err != nil {
		return nil, err
	}

	return &aggregator{
		operationsCounter:   metricsFactory.Counter(metrics.Options{Name: "sampling_operations"}),
		servicesCounter:     metricsFactory.Counter(metrics.Options{Name: "sampling_services"}),
		currentThroughput:   make(serviceOperationThroughput),
		aggregationInterval: options.CalculationInterval,
		postAggregator:      postAgg,
		storage:             store,
		stop:                make(chan struct{}),
	}, nil
}

func (a *aggregator) runAggregationLoop() {
	ticker := time.NewTicker(a.aggregationInterval)

	// NB: the first tick will be slightly delayed by the initializeThroughput call.
	a.postAggregator.lastCheckedTime = time.Now().Add(a.postAggregator.Delay * -1)
	a.postAggregator.initializeThroughput(a.postAggregator.lastCheckedTime)
	for {
		select {
		case <-ticker.C:
			a.Lock()
			a.saveThroughput()
			a.currentThroughput = make(serviceOperationThroughput)
			a.postAggregator.runCalculation()
			a.Unlock()
		case <-a.stop:
			ticker.Stop()
			return
		}
	}
}

func (a *aggregator) saveThroughput() {
	totalOperations := 0
	var throughputSlice []*model.Throughput
	for _, opThroughput := range a.currentThroughput {
		totalOperations += len(opThroughput)
		for _, throughput := range opThroughput {
			throughputSlice = append(throughputSlice, throughput)
		}
	}
	a.operationsCounter.Inc(int64(totalOperations))
	a.servicesCounter.Inc(int64(len(a.currentThroughput)))
	a.storage.InsertThroughput(throughputSlice)
}

func (a *aggregator) RecordThroughput(service, operation string, samplerType span_model.SamplerType, probability float64) {
	a.Lock()
	defer a.Unlock()
	if _, ok := a.currentThroughput[service]; !ok {
		a.currentThroughput[service] = make(map[string]*model.Throughput)
	}
	throughput, ok := a.currentThroughput[service][operation]
	if !ok {
		throughput = &model.Throughput{
			Service:       service,
			Operation:     operation,
			Probabilities: make(map[string]struct{}),
		}
		a.currentThroughput[service][operation] = throughput
	}
	probStr := TruncateFloat(probability)
	if len(throughput.Probabilities) != maxProbabilities {
		throughput.Probabilities[probStr] = struct{}{}
	}
	// Only if we see probabilistically sampled root spans do we increment the throughput counter,
	// for lowerbound sampled spans, we don't increment at all but we still save a count of 0 as
	// the throughput so that the adaptive sampling processor is made aware of the endpoint.
	if samplerType == span_model.SamplerTypeProbabilistic {
		throughput.Count++
	}
}

func (a *aggregator) Start() {
	a.postAggregator.Start()
	a.runBackground(a.runAggregationLoop)
}

func (a *aggregator) Close() error {
	var errs []error
	if err := a.postAggregator.Close(); err != nil {
		errs = append(errs, err)
	}
	// if a.stop != nil {
	// 	close(a.stop)
	// }
	a.bgFinished.Wait()
	return errors.Join(errs...)
}

func (a *aggregator) runBackground(f func()) {
	a.bgFinished.Add(1)
	go func() {
		f()
		a.bgFinished.Done()
	}()
}
