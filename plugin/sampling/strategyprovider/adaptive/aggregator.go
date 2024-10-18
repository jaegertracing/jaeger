// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/samplingstrategy"
	span_model "github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/hostname"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/sampling/leaderelection"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
)

const (
	maxProbabilities = 10
)

// aggregator is a kind of trace processor that watches for root spans
// and calculates how many traces per service / per endpoint are being
// produced. It periodically flushes these stats ("throughput") to storage.
//
// It also invokes PostAggregator which actually computes adaptive sampling
// probabilities based on the observed throughput.
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
func NewAggregator(options Options, logger *zap.Logger, metricsFactory metrics.Factory, participant leaderelection.ElectionParticipant, store samplingstore.Store) (samplingstrategy.Aggregator, error) {
	hostIdentifier, err := hostname.AsIdentifier()
	if err != nil {
		return nil, err
	}
	logger.Info("Using unique participantName in adaptive sampling", zap.String("participantName", hostIdentifier))

	postAggregator, err := newPostAggregator(options, hostIdentifier, store, participant, metricsFactory, logger)
	if err != nil {
		return nil, err
	}

	return &aggregator{
		operationsCounter:   metricsFactory.Counter(metrics.Options{Name: "sampling_operations"}),
		servicesCounter:     metricsFactory.Counter(metrics.Options{Name: "sampling_services"}),
		currentThroughput:   make(serviceOperationThroughput),
		aggregationInterval: options.CalculationInterval,
		postAggregator:      postAggregator,
		storage:             store,
		stop:                make(chan struct{}),
	}, nil
}

func (a *aggregator) runAggregationLoop() {
	ticker := time.NewTicker(a.aggregationInterval)
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

	a.bgFinished.Add(1)
	go func() {
		a.runAggregationLoop()
		a.bgFinished.Done()
	}()
}

func (a *aggregator) Close() error {
	close(a.stop)
	a.bgFinished.Wait()
	return nil
}

func (a *aggregator) HandleRootSpan(span *span_model.Span, logger *zap.Logger) {
	// simply checking parentId to determine if a span is a root span is not sufficient. However,
	// we can be sure that only a root span will have sampler tags.
	if span.ParentSpanID() != span_model.NewSpanID(0) {
		return
	}
	service := span.Process.ServiceName
	if service == "" || span.OperationName == "" {
		return
	}
	samplerType, samplerParam := span.GetSamplerParams(logger)
	if samplerType == span_model.SamplerTypeUnrecognized {
		return
	}
	a.RecordThroughput(service, span.OperationName, samplerType, samplerParam)
}
