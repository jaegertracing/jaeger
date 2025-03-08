// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/leaderelection"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive/calculationstrategy"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

const (
	getThroughputErrMsg = "failed to get throughput from storage"

	// The number of past entries for samplingCache the leader keeps in memory
	serviceCacheSize = 25

	defaultResourceName = "sampling_store_leader"
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

// PostAggregator retrieves service throughput over a lookback interval and calculates sampling probabilities
// per operation such that each operation is sampled at a specified target QPS. It achieves this by
// retrieving discrete buckets of operation throughput and doing a weighted average of the throughput
// and generating a probability to match the targetQPS.
type PostAggregator struct {
	sync.RWMutex
	Options

	electionParticipant leaderelection.ElectionParticipant
	storage             samplingstore.Store
	logger              *zap.Logger
	hostname            string

	// probabilities contains the latest calculated sampling probabilities for service operations.
	probabilities model.ServiceOperationProbabilities

	// qps contains the latest calculated qps for service operations; the calculation is essentially
	// throughput / CalculationInterval.
	qps model.ServiceOperationQPS

	// throughputs is an  array (of `AggregationBuckets` size) that stores the aggregated throughput.
	// The latest throughput is stored at the head of the slice.
	throughputs []*throughputBucket

	weightVectorCache *WeightVectorCache

	probabilityCalculator calculationstrategy.ProbabilityCalculator

	serviceCache []SamplingCache

	shutdown chan struct{}

	operationsCalculatedGauge     metrics.Gauge
	calculateProbabilitiesLatency metrics.Timer
	lastCheckedTime               time.Time
}

// newPostAggregator creates a new sampling postAggregator that generates sampling rates for service operations.
func newPostAggregator(
	opts Options,
	hostname string,
	storage samplingstore.Store,
	electionParticipant leaderelection.ElectionParticipant,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*PostAggregator, error) {
	if opts.CalculationInterval == 0 || opts.AggregationBuckets == 0 {
		return nil, errNonZero
	}
	if opts.BucketsForCalculation < 1 {
		return nil, errBucketsForCalculation
	}
	metricsFactory = metricsFactory.Namespace(metrics.NSOptions{Name: "adaptive_sampling_processor"})
	return &PostAggregator{
		Options:             opts,
		storage:             storage,
		probabilities:       make(model.ServiceOperationProbabilities),
		qps:                 make(model.ServiceOperationQPS),
		hostname:            hostname,
		logger:              logger,
		electionParticipant: electionParticipant,
		// TODO make weightsCache and probabilityCalculator configurable
		weightVectorCache:             NewWeightVectorCache(),
		probabilityCalculator:         calculationstrategy.NewPercentageIncreaseCappedCalculator(1.0),
		serviceCache:                  []SamplingCache{},
		operationsCalculatedGauge:     metricsFactory.Gauge(metrics.Options{Name: "operations_calculated"}),
		calculateProbabilitiesLatency: metricsFactory.Timer(metrics.TimerOptions{Name: "calculate_probabilities"}),
		shutdown:                      make(chan struct{}),
	}, nil
}

// Start initializes and starts the sampling postAggregator which regularly calculates sampling probabilities.
func (p *PostAggregator) Start() error {
	p.logger.Info("starting adaptive sampling postAggregator")
	// NB: the first tick will be slightly delayed by the initializeThroughput call.
	p.lastCheckedTime = time.Now().Add(p.Delay * -1)
	p.initializeThroughput(p.lastCheckedTime)
	return nil
}

func (p *PostAggregator) isLeader() bool {
	return p.electionParticipant.IsLeader()
}

// addJitter adds a random amount of time. Without jitter, if the host holding the leader
// lock were to die, then all other collectors can potentially wait for a full cycle before
// trying to acquire the lock. With jitter, we can reduce the average amount of time before a
// new leader is elected. Furthermore, jitter can be used to spread out read load on storage.
func addJitter(jitterAmount time.Duration) time.Duration {
	return (jitterAmount / 2) + time.Duration(rand.Int63n(int64(jitterAmount/2)))
}

func (p *PostAggregator) runCalculation() {
	endTime := time.Now().Add(p.Delay * -1)
	startTime := p.lastCheckedTime
	throughput, err := p.storage.GetThroughput(startTime, endTime)
	if err != nil {
		p.logger.Error(getThroughputErrMsg, zap.Error(err))
		return
	}
	aggregatedThroughput := p.aggregateThroughput(throughput)
	p.prependThroughputBucket(&throughputBucket{
		throughput: aggregatedThroughput,
		interval:   endTime.Sub(startTime),
		endTime:    endTime,
	})
	p.lastCheckedTime = endTime
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
		// NB: This has the potential of running into a race condition if the CalculationInterval
		// is set to an extremely low value. The worst case scenario is that probabilities is calculated
		// and swapped more than once before generateStrategyResponses() and saveProbabilities() are called.
		// This will result in one or more batches of probabilities not being saved which is completely
		// fine. This race condition should not ever occur anyway since the calculation interval will
		// be way longer than the time to run the calculations.

		p.calculateProbabilitiesLatency.Record(time.Since(startTime))
		p.saveProbabilitiesAndQPS()
	}
}

func (p *PostAggregator) saveProbabilitiesAndQPS() {
	p.RLock()
	defer p.RUnlock()
	if err := p.storage.InsertProbabilitiesAndQPS(p.hostname, p.probabilities, p.qps); err != nil {
		p.logger.Warn("could not save probabilities", zap.Error(err))
	}
}

func (p *PostAggregator) prependThroughputBucket(bucket *throughputBucket) {
	p.throughputs = append([]*throughputBucket{bucket}, p.throughputs...)
	if len(p.throughputs) > p.AggregationBuckets {
		p.throughputs = p.throughputs[0:p.AggregationBuckets]
	}
}

// aggregateThroughput aggregates operation throughput from different buckets into one.
// All input buckets represent a single time range, but there are many of them because
// they are all independently generated by different collector instances from inbound span traffic.
func (*PostAggregator) aggregateThroughput(throughputs []*model.Throughput) serviceOperationThroughput {
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
			copyThroughput := model.Throughput{
				Service:       throughput.Service,
				Operation:     throughput.Operation,
				Count:         throughput.Count,
				Probabilities: copySet(throughput.Probabilities),
			}
			aggregatedThroughput[service][operation] = &copyThroughput
		}
	}
	return aggregatedThroughput
}

func copySet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for key := range in {
		out[key] = struct{}{}
	}
	return out
}

func (p *PostAggregator) initializeThroughput(endTime time.Time) {
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
func (p *PostAggregator) throughputToQPS() serviceOperationQPS {
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
func (p *PostAggregator) calculateWeightedQPS(allQPS []float64) float64 {
	if len(allQPS) == 0 {
		return 0
	}
	weights := p.weightVectorCache.GetWeights(len(allQPS))
	var qps float64
	for i := 0; i < len(allQPS); i++ {
		qps += allQPS[i] * weights[i]
	}
	return qps
}

func (p *PostAggregator) prependServiceCache() {
	p.serviceCache = append([]SamplingCache{make(SamplingCache)}, p.serviceCache...)
	if len(p.serviceCache) > serviceCacheSize {
		p.serviceCache = p.serviceCache[0:serviceCacheSize]
	}
}

func (p *PostAggregator) calculateProbabilitiesAndQPS() (model.ServiceOperationProbabilities, model.ServiceOperationQPS) {
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

func (p *PostAggregator) calculateProbability(service, operation string, qps float64) float64 {
	// TODO: is this method overly expensive to run in a loop?
	oldProbability := p.InitialSamplingProbability
	p.RLock()
	if opProbabilities, ok := p.probabilities[service]; ok {
		if probability, ok := opProbabilities[operation]; ok {
			oldProbability = probability
		}
	}
	latestThroughput := p.throughputs[0].throughput
	p.RUnlock()

	usingAdaptiveSampling := p.isUsingAdaptiveSampling(oldProbability, service, operation, latestThroughput)
	p.serviceCache[0].Set(service, operation, &SamplingCacheEntry{
		Probability:   oldProbability,
		UsingAdaptive: usingAdaptiveSampling,
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
	return math.Min(1.0, math.Max(p.MinSamplingProbability, newProbability))
}

// is actual value within p.DeltaTolerance percentage of expected value.
func (p *PostAggregator) withinTolerance(actual, expected float64) bool {
	return math.Abs(actual-expected)/expected < p.DeltaTolerance
}

// merge (union) string set p2 into string set p1
func merge(p1 map[string]struct{}, p2 map[string]struct{}) map[string]struct{} {
	for k := range p2 {
		p1[k] = struct{}{}
	}
	return p1
}

func (p *PostAggregator) isUsingAdaptiveSampling(
	probability float64,
	service string,
	operation string,
	throughput serviceOperationThroughput,
) bool {
	if p.IgnoreSamplerTags {
		return true
	}
	if floatEquals(probability, p.InitialSamplingProbability) {
		// If the service is seen for the first time, assume it's using adaptive sampling (ie prob == initialProb).
		// Even if this isn't the case, the next time around this loop, the newly calculated probability will not equal
		// the initialProb so the logic will fall through.
		return true
	}
	// if the previous probability can be found in the set of observed values
	// in the latest time bucket then assume service is respecting adaptive sampling.
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
			return e.UsingAdaptive && !floatEquals(e.Probability, p.InitialSamplingProbability)
		}
	}
	return false
}
