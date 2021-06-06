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

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

const (
	maxProbabilities = 10
)

// Aggregator defines an interface used to aggregate operation throughput.
type Aggregator interface {
	// RecordThroughput records throughput for an operation for aggregation.
	RecordThroughput(service, operation, samplerType string, probability float64)

	// Start starts aggregating operation throughput.
	Start()

	// Stop stops the aggregator from aggregating throughput.
	Stop()
}

type aggregator struct {
	sync.Mutex

	operationsCounter   metrics.Counter
	servicesCounter     metrics.Counter
	currentThroughput   serviceOperationThroughput
	aggregationInterval time.Duration
	storage             samplingstore.Store
	stop                chan struct{}
}

// NewAggregator creates a throughput aggregator that simply emits metrics
// about the number of operations seen over the aggregationInterval.
func NewAggregator(metricsFactory metrics.Factory, interval time.Duration, storage samplingstore.Store) Aggregator {
	return &aggregator{
		operationsCounter:   metricsFactory.Counter(metrics.Options{Name: "sampling_operations"}),
		servicesCounter:     metricsFactory.Counter(metrics.Options{Name: "sampling_services"}),
		currentThroughput:   make(serviceOperationThroughput),
		aggregationInterval: interval,
		storage:             storage,
		stop:                make(chan struct{}),
	}
}

func (a *aggregator) runAggregationLoop() {
	ticker := time.NewTicker(a.aggregationInterval)
	for {
		select {
		case <-ticker.C:
			a.Lock()
			a.saveThroughput()
			a.currentThroughput = make(serviceOperationThroughput)
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

func (a *aggregator) RecordThroughput(service, operation, samplerType string, probability float64) {
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
	if samplerType == jaeger.SamplerTypeProbabilistic {
		throughput.Count++
	}
}

func (a *aggregator) Start() {
	go a.runAggregationLoop()
}

func (a *aggregator) Stop() {
	close(a.stop)
}
