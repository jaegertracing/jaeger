// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	span_model "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/leaderelection"
	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/pkg/hostname"
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

	operationsCounter   api.Counter
	servicesCounter     api.Counter
	currentThroughput   serviceOperationThroughput
	postAggregator      *PostAggregator
	aggregationInterval time.Duration
	storage             samplingstore.Store
	stop                chan struct{}
	bgFinished          sync.WaitGroup
}

// NewAggregator creates a throughput aggregator that simply emits metrics
// about the number of operations seen over the aggregationInterval.
func NewAggregator(options Options, logger *zap.Logger, metricsFactory api.Factory, participant leaderelection.ElectionParticipant, store samplingstore.Store) (samplingstrategy.Aggregator, error) {
	hostId, err := hostname.AsIdentifier()
	if err != nil {
		return nil, err
	}
	logger.Info("Using unique participantName in adaptive sampling", zap.String("participantName", hostId))

	postAggregator, err := newPostAggregator(options, hostId, store, participant, metricsFactory, logger)
	if err != nil {
		return nil, err
	}

	return &aggregator{
		operationsCounter:   metricsFactory.Counter(api.Options{Name: "sampling_operations"}),
		servicesCounter:     metricsFactory.Counter(api.Options{Name: "sampling_services"}),
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

func (a *aggregator) HandleRootSpan(span *span_model.Span) {
	// simply checking parentId to determine if a span is a root span is not sufficient. However,
	// we can be sure that only a root span will have sampler tags.
	if span.ParentSpanID() != span_model.NewSpanID(0) {
		return
	}
	service := span.Process.ServiceName
	if service == "" || span.OperationName == "" {
		return
	}
	samplerType, samplerParam := getSamplerParams(span, a.postAggregator.logger)
	if samplerType == span_model.SamplerTypeUnrecognized {
		return
	}
	a.RecordThroughput(service, span.OperationName, samplerType, samplerParam)
}

// GetSamplerParams returns the sampler.type and sampler.param value if they are valid.
func getSamplerParams(s *span_model.Span, logger *zap.Logger) (span_model.SamplerType, float64) {
	samplerType := s.GetSamplerType()
	if samplerType == span_model.SamplerTypeUnrecognized {
		return span_model.SamplerTypeUnrecognized, 0
	}
	tag, ok := span_model.KeyValues(s.Tags).FindByKey(span_model.SamplerParamKey)
	if !ok {
		return span_model.SamplerTypeUnrecognized, 0
	}
	samplerParam, err := samplerParamToFloat(tag)
	if err != nil {
		logger.
			With(zap.String("traceID", s.TraceID.String())).
			With(zap.String("spanID", s.SpanID.String())).
			Warn("sampler.param tag is not a number", zap.Any("tag", tag))
		return span_model.SamplerTypeUnrecognized, 0
	}
	return samplerType, samplerParam
}

func samplerParamToFloat(samplerParamTag span_model.KeyValue) (float64, error) {
	// The param could be represented as a string, an int, or a float
	switch samplerParamTag.VType {
	case span_model.Float64Type:
		return samplerParamTag.Float64(), nil
	case span_model.Int64Type:
		return float64(samplerParamTag.Int64()), nil
	default:
		return strconv.ParseFloat(samplerParamTag.AsString(), 64)
	}
}
