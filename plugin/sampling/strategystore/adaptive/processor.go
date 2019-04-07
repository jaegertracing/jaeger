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

package adaptive

import (
	"errors"
	"io"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	ss "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	jio "github.com/jaegertracing/jaeger/pkg/io"
	"github.com/jaegertracing/jaeger/plugin/sampling/internal/calculationstrategy"
	"github.com/jaegertracing/jaeger/plugin/sampling/internal/leaderelection"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

const (
	maxSamplingProbability = 1.0

	getThroughputErrMsg = "failed to get throughput from storage"

	defaultFollowerProbabilityInterval = 20 * time.Second

	// The number of past entries for samplingCache the leader keeps in memory
	serviceCacheSize = 25
)

var (
	errNonZero               = errors.New("CalculationInterval and AggregationBuckets must be greater than 0")
	errBucketsForCalculation = errors.New("BucketsForCalculation cannot be less than 1")
)

// nested map: service -> operation -> throughput.
type serviceOperationThroughput map[string]map[string]*model.Throughput

func (t serviceOperationThroughput) get(service, operation string) (*model.Throughput, bool) {
	svcThroughput, ok := t[service]
	if ok {
		v, ok := svcThroughput[operation]
		return v, ok
	}
	return nil, false
}

// nested map: service -> operation -> buckets of QPS values.
type serviceOperationQPS map[string]map[string][]float64

type throughputBucket struct {
	throughput serviceOperationThroughput
	interval   time.Duration
	endTime    time.Time
}

// processor retrieves service throughput over a look back interval and calculates sampling probabilities
// per operation such that each operation is sampled at a specified target QPS. It achieves this by
// retrieving discrete buckets of operation throughput and doing a weighted average of the throughput
// and generating a probability to match the targetQPS.
type processor struct {
	sync.RWMutex
	Options

	electionParticipant leaderelection.ElectionParticipant
	storage             samplingstore.Store
	logger              *zap.Logger
	hostname            string

	// probabilities contains the latest calculated sampling probabilities for service operations.
	probabilities model.ServiceOperationProbabilities

	// qps contains the latest calculated qps for service operations; the calculation is essentially
	// throughput / AggregationInterval.
	qps model.ServiceOperationQPS

	// throughputs is an  array (of `AggregationBuckets` size) that stores the aggregated throughput.
	// The latest throughput is stored at the head of the slice.
	throughputs []*throughputBucket

	// strategyResponses is the cache of the sampling strategies for every service, in Thrift format.
	// TODO change this to work with protobuf model instead, to support gRPC endpoint.
	strategyResponses map[string]*sampling.SamplingStrategyResponse

	weightVectorCache *weightVectorCache

	probabilityCalculator calculationstrategy.ProbabilityCalculator

	// followerRefreshInterval determines how often the follower processor updates its probabilities.
	// Given only the leader writes probabilities, the followers need to fetch the probabilities into
	// cache.
	followerRefreshInterval time.Duration

	serviceCache []samplingCache

	shutdown chan struct{}

	operationsCalculatedGauge     metrics.Gauge
	calculateProbabilitiesLatency metrics.Timer
}

// NewProcessor creates a new sampling processor that generates sampling rates for service operations
func NewProcessor(
	opts Options,
	hostname string,
	storage samplingstore.Store,
	electionParticipant leaderelection.ElectionParticipant,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (ss.StrategyStore, error) {
	if opts.CalculationInterval == 0 || opts.AggregationBuckets == 0 {
		return nil, errNonZero
	}
	if opts.BucketsForCalculation < 1 {
		return nil, errBucketsForCalculation
	}
	metricsFactory = metricsFactory.Namespace(metrics.NSOptions{Name: "adaptive_sampling_processor"})
	return &processor{
		Options:             opts,
		storage:             storage,
		probabilities:       make(model.ServiceOperationProbabilities),
		qps:                 make(model.ServiceOperationQPS),
		hostname:            hostname,
		strategyResponses:   make(map[string]*sampling.SamplingStrategyResponse),
		logger:              logger,
		electionParticipant: electionParticipant,
		// TODO make weightsCache and probabilityCalculator configurable
		weightVectorCache:             newWeightVectorCache(),
		probabilityCalculator:         calculationstrategy.NewPercentageIncreaseCappedCalculator(1.0),
		followerRefreshInterval:       defaultFollowerProbabilityInterval,
		serviceCache:                  []samplingCache{},
		operationsCalculatedGauge:     metricsFactory.Gauge(metrics.Options{Name: "operations_calculated"}),
		calculateProbabilitiesLatency: metricsFactory.Timer(metrics.TimerOptions{Name: "calculate_probabilities"}),
	}, nil
}

// GetSamplingStrategy implements Thrift endpoint for retrieving sampling strategy for a service.
func (p *processor) GetSamplingStrategy(service string) (*sampling.SamplingStrategyResponse, error) {
	p.RLock()
	defer p.RUnlock()
	if strategy, ok := p.strategyResponses[service]; ok {
		return strategy, nil
	}
	return p.generateDefaultSamplingStrategyResponse(), nil
}

// Start initializes and starts the sampling processor which regularly calculates sampling probabilities.
func (p *processor) Start() error {
	p.logger.Info("starting adaptive sampling processor")
	p.shutdown = make(chan struct{})
	if starter, ok := p.electionParticipant.(jio.Starter); ok {
		starter.Start()
	}
	p.loadProbabilities()
	p.generateStrategyResponses()
	go p.runCalculationLoop()
	go p.runUpdateProbabilitiesLoop()
	return nil
}

// Close stops the processor from calculating probabilities.
func (p *processor) Close() error {
	p.logger.Info("stopping adaptive sampling processor")
	if closer, ok := p.electionParticipant.(io.Closer); ok {
		closer.Close()
	}
	close(p.shutdown)
	return nil
}

func (p *processor) loadProbabilities() {
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
func (p *processor) runUpdateProbabilitiesLoop() {
	addJitter(p.followerRefreshInterval)
	ticker := time.NewTicker(p.followerRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Only load probabilities if this processor doesn't hold the leader lock
			if !p.isLeader() {
				p.loadProbabilities()
				p.generateStrategyResponses()
			}
		case <-p.shutdown:
			return
		}
	}
}

func (p *processor) isLeader() bool {
	return p.electionParticipant.IsLeader()
}

// addJitter sleeps for a random amount of time. Without jitter, if the host holding the leader
// lock were to die, then all other collectors can potentially wait for a full cycle before
// trying to acquire the lock. With jitter, we can reduce the average amount of time before a
// new leader is elected. Furthermore, jitter can be used to spread out read load on storage.
func addJitter(jitterAmount time.Duration) {
	delay := (jitterAmount / 2) + time.Duration(rand.Int63n(int64(jitterAmount/2)))
	time.Sleep(delay)
}

func (p *processor) runCalculationLoop() {
	lastCheckedTime := time.Now().Add(p.Delay * -1)
	p.initializeThroughput(lastCheckedTime)
	// NB: the first tick will be slightly delayed by the initializeThroughput call.
	ticker := time.NewTicker(p.CalculationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			endTime := time.Now().Add(p.Delay * -1)
			startTime := lastCheckedTime
			throughput, err := p.storage.GetThroughput(startTime, endTime)
			if err != nil {
				p.logger.Error(getThroughputErrMsg, zap.Error(err))
				break
			}
			aggregatedThroughput := p.aggregateThroughput(throughput)
			p.prependThroughputBucket(&throughputBucket{
				throughput: aggregatedThroughput,
				interval:   endTime.Sub(startTime),
				endTime:    endTime,
			})
			lastCheckedTime = endTime
			// Load the latest throughput so that if this host ever becomes leader, it
			// has the throughput ready in memory. However, only run the actual calculations
			// if this host becomes leader.
			// TODO fill the throughput buffer only when we're leader
			if p.isLeader() {
				startTime := time.Now()
				probabilities, qps := p.calculateProbabilitiesAndQPS()
				p.Lock()
				p.probabilities = probabilities
				p.qps = qps
				p.Unlock()
				// NB: This has the potential of running into a race condition if the AggregationInterval
				// is set to an extremely low value. The worst case scenario is that probabilities is calculated
				// and swapped more than once before generateStrategyResponses() and saveProbabilities() are called.
				// This will result in one or more batches of probabilities not being saved which is completely
				// fine. This race condition should not ever occur anyway since the calculation interval will
				// be way longer than the time to run the calculations.
				p.generateStrategyResponses()
				p.calculateProbabilitiesLatency.Record(time.Since(startTime))
				go p.saveProbabilitiesAndQPS()
			}
		case <-p.shutdown:
			return
		}
	}
}

func (p *processor) saveProbabilitiesAndQPS() {
	p.RLock()
	defer p.RUnlock()
	if err := p.storage.InsertProbabilitiesAndQPS(p.hostname, p.probabilities, p.qps); err != nil {
		p.logger.Warn("could not save probabilities", zap.Error(err))
	}
}

func (p *processor) prependThroughputBucket(bucket *throughputBucket) {
	p.throughputs = append([]*throughputBucket{bucket}, p.throughputs...)
	if len(p.throughputs) > p.AggregationBuckets {
		p.throughputs = p.throughputs[0:p.AggregationBuckets]
	}
}

// aggregateThroughput aggregates operation throughput from different buckets into one.
// All input buckets represent a single time range, but there are many of them because
// they are all independently generated by different collector instances from inbound span traffic.
func (p *processor) aggregateThroughput(throughputs []*model.Throughput) serviceOperationThroughput {
	aggregatedThroughput := make(serviceOperationThroughput)
	for _, throughput := range throughputs {
		service := throughput.Service
		operation := throughput.Operation
		if _, ok := aggregatedThroughput[service]; !ok {
			aggregatedThroughput[service] = make(map[string]*model.Throughput)
		}
		if t, ok := aggregatedThroughput[service][operation]; ok {
			t.Count += throughput.Count
			t.Probabilities = merge(t.Probabilities, throughput.Probabilities)
		} else {
			aggregatedThroughput[service][operation] = throughput
		}
	}
	return aggregatedThroughput
}

func (p *processor) initializeThroughput(endTime time.Time) {
	for i := 0; i < p.AggregationBuckets; i++ {
		startTime := endTime.Add(p.CalculationInterval * -1)
		throughput, err := p.storage.GetThroughput(startTime, endTime)
		if err != nil && p.logger != nil {
			p.logger.Error(getThroughputErrMsg, zap.Error(err))
			return
		}
		if len(throughput) == 0 {
			return
		}
		aggregatedThroughput := p.aggregateThroughput(throughput)
		p.throughputs = append(p.throughputs, &throughputBucket{
			throughput: aggregatedThroughput,
			interval:   p.CalculationInterval,
			endTime:    endTime,
		})
		endTime = startTime
	}
}

// throughputToQPS converts raw throughput counts for all accumulated buckets to QPS values.
func (p *processor) throughputToQPS() serviceOperationQPS {
	// TODO previous qps buckets have already been calculated, just need to calculate latest batch
	// and append them where necessary and throw out the oldest batch.
	// Edge case #buckets < p.AggregationBuckets, then we shouldn't throw out
	qps := make(serviceOperationQPS)
	for _, bucket := range p.throughputs {
		for svc, operations := range bucket.throughput {
			if _, ok := qps[svc]; !ok {
				qps[svc] = make(map[string][]float64)
			}
			for op, throughput := range operations {
				if len(qps[svc][op]) >= p.BucketsForCalculation {
					continue
				}
				qps[svc][op] = append(qps[svc][op], calculateQPS(throughput.Count, bucket.interval))
			}
		}
	}
	return qps
}

func calculateQPS(count int64, interval time.Duration) float64 {
	seconds := float64(interval) / float64(time.Second)
	return float64(count) / seconds
}

// calculateWeightedQPS calculates the weighted qps of the slice allQPS where weights are biased
// towards more recent qps. This function assumes that the most recent qps is at the head of the slice.
func (p *processor) calculateWeightedQPS(allQPS []float64) float64 {
	if len(allQPS) == 0 {
		return 0
	}
	weights := p.weightVectorCache.getWeights(len(allQPS))
	var qps float64
	for i := 0; i < len(allQPS); i++ {
		qps += allQPS[i] * weights[i]
	}
	return qps
}

func (p *processor) prependServiceCache() {
	p.serviceCache = append([]samplingCache{make(samplingCache)}, p.serviceCache...)
	if len(p.serviceCache) > serviceCacheSize {
		p.serviceCache = p.serviceCache[0:serviceCacheSize]
	}
}

func (p *processor) calculateProbabilitiesAndQPS() (model.ServiceOperationProbabilities, model.ServiceOperationQPS) {
	p.prependServiceCache()
	retProbabilities := make(model.ServiceOperationProbabilities)
	retQPS := make(model.ServiceOperationQPS)
	svcOpQPS := p.throughputToQPS()
	totalOperations := int64(0)
	for svc, opQPS := range svcOpQPS {
		if _, ok := retProbabilities[svc]; !ok {
			retProbabilities[svc] = make(map[string]float64)
		}
		if _, ok := retQPS[svc]; !ok {
			retQPS[svc] = make(map[string]float64)
		}
		for op, qps := range opQPS {
			totalOperations++
			avgQPS := p.calculateWeightedQPS(qps)
			retQPS[svc][op] = avgQPS
			retProbabilities[svc][op] = p.calculateProbability(svc, op, avgQPS)
		}
	}
	p.operationsCalculatedGauge.Update(totalOperations)
	return retProbabilities, retQPS
}

func (p *processor) calculateProbability(service, operation string, qps float64) float64 {
	oldProbability := p.InitialSamplingProbability
	// TODO: is this loop overly expensive?
	p.RLock()
	if opProbabilities, ok := p.probabilities[service]; ok {
		if probability, ok := opProbabilities[operation]; ok {
			oldProbability = probability
		}
	}
	latestThroughput := p.throughputs[0].throughput
	p.RUnlock()

	usingAdaptiveSampling := p.isUsingAdaptiveSampling(oldProbability, service, operation, latestThroughput)
	p.serviceCache[0].Set(service, operation, &samplingCacheEntry{
		probability:   oldProbability,
		usingAdaptive: usingAdaptiveSampling,
	})

	// Short circuit if the qps is close enough to targetQPS or if the service doesn't appear to be using
	// adaptive sampling.
	if p.withinTolerance(qps, p.TargetSamplesPerSecond) || !usingAdaptiveSampling {
		return oldProbability
	}
	var newProbability float64
	if floatEquals(qps, 0) {
		// Edge case; we double the sampling probability if the QPS is 0 so that we force the service
		// to at least sample one span probabilistically.
		newProbability = oldProbability * 2.0
	} else {
		newProbability = p.probabilityCalculator.Calculate(p.TargetSamplesPerSecond, qps, oldProbability)
	}
	return math.Min(maxSamplingProbability, math.Max(p.MinSamplingProbability, newProbability))
}

// is actual value within p.DeltaTolerance percentage of expected value.
func (p *processor) withinTolerance(actual, expected float64) bool {
	return math.Abs(actual-expected)/expected < p.DeltaTolerance
}

// merge (union) string set p2 into string set p1
func merge(p1 map[string]struct{}, p2 map[string]struct{}) map[string]struct{} {
	for k := range p2 {
		p1[k] = struct{}{}
	}
	return p1
}

func (p *processor) isUsingAdaptiveSampling(
	probability float64,
	service string,
	operation string,
	throughput serviceOperationThroughput,
) bool {
	if floatEquals(probability, p.InitialSamplingProbability) {
		// If the service is seen for the first time, assume it's using adaptive sampling (ie prob == initialProb).
		// Even if this isn't the case, the next time around this loop, the newly calculated probability will not equal
		// the initialProb so the logic will fall through.
		return true
	}
	if opThroughput, ok := throughput.get(service, operation); ok {
		f := truncateFloat(probability)
		_, ok := opThroughput.Probabilities[f]
		return ok
	}
	// By this point, we know that there's no recorded throughput for this operation for this round
	// of calculation. Check the previous bucket to see if this operation was using adaptive sampling
	// before.
	if len(p.serviceCache) > 1 {
		if e := p.serviceCache[1].Get(service, operation); e != nil {
			return e.usingAdaptive && !floatEquals(e.probability, p.InitialSamplingProbability)
		}
	}
	return false
}

// generateStrategyResponses generates and caches SamplingStrategyResponse from the calculated sampling probabilities.
func (p *processor) generateStrategyResponses() {
	p.RLock()
	strategies := make(map[string]*sampling.SamplingStrategyResponse)
	for svc, opProbabilities := range p.probabilities {
		opStrategies := make([]*sampling.OperationSamplingStrategy, len(opProbabilities))
		var idx int
		for op, probability := range opProbabilities {
			opStrategies[idx] = &sampling.OperationSamplingStrategy{
				Operation: op,
				ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
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

func (p *processor) generateDefaultSamplingStrategyResponse() *sampling.SamplingStrategyResponse {
	return &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		OperationSampling: &sampling.PerOperationSamplingStrategies{
			DefaultSamplingProbability:       p.InitialSamplingProbability,
			DefaultLowerBoundTracesPerSecond: p.MinSamplesPerSecond,
		},
	}
}
