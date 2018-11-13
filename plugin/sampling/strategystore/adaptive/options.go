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
	"flag"
	"time"

	"github.com/spf13/viper"
)

const (
	targetQPS                    = "sampling.target-qps"
	equivalenceThreshold         = "sampling.equivalence-threshold"
	lookbackQPSCount             = "sampling.lookback-qps-count"
	calculationInterval          = "sampling.calculation-interval"
	lookbackInternval            = "sampling.lookback-interval"
	delay                        = "sampling.delay"
	defaultSamplingProbability   = "sampling.default-sampling-probability"
	minSamplingProbability       = "sampling.min-sampling-probability"
	lowerBoundTracesPerSecond    = "sampling.lower-bound-traces-per-second"
	leaderLeaseRefreshInterval   = "sampling.leader-lease-refresh-interval"
	followerLeaseRefreshInterval = "sampling.follower-lease-refresh-interval"

	defaultTargetQPS                    = 1
	defaultEquivalenceThreshold         = 0.3
	defaultLookbackQPSCount             = 1
	defaultCalculationInterval          = time.Minute
	defaultLookbackInterval             = time.Minute * 10
	defaultDelay                        = time.Minute * 2
	defaultDefaultSamplingProbability   = 0.001
	defaultMinSamplingProbability       = 0.00001                                      // once in 100 thousand requests
	defaultLowerBoundTracesPerSecond    = 1.0 / (1 * float64(time.Minute/time.Second)) // once every 1 minute
	defaultLeaderLeaseRefreshInterval   = 5 * time.Second
	defaultFollowerLeaseRefreshInterval = 60 * time.Second

	samplingLock = "sampling_lock"
)

// Options holds configuration for the adaptive sampling strategy store.
type Options struct {
	// TargetQPS is the target sampled qps for all operations.
	TargetQPS float64

	// QPSEquivalenceThreshold is the acceptable amount of deviation for the operation QPS from the `targetQPS`,
	// ie. [targetQPS-equivalenceThreshold, targetQPS+equivalenceThreshold] is the acceptable targetQPS range.
	// Increase this to reduce the amount of fluctuation in the probability calculation.
	QPSEquivalenceThreshold float64

	// LookbackQPSCount determines how many previous operation QPS are used in calculating the weighted QPS,
	// ie. if LookbackQPSCount is 1, the only the most recent QPS will be used in calculating the weighted QPS.
	LookbackQPSCount int

	// CalculationInterval determines the interval each bucket represents, ie. if an interval is
	// 1 minute, the bucket will contain 1 minute of throughput data for all services.
	CalculationInterval time.Duration

	// LookbackInterval is the total amount of throughput data used to calculate probabilities.
	LookbackInterval time.Duration

	// Delay is the amount of time to delay probability generation by, ie. if the calculationInterval
	// is 1 minute, the number of buckets is 10, and the delay is 2 minutes, then at one time
	// we'll have [now()-12,now()-2] range of throughput data in memory to base the calculations
	// off of.
	Delay time.Duration

	// DefaultSamplingProbability is the initial sampling probability for all new operations.
	DefaultSamplingProbability float64

	// MinSamplingProbability is the minimum sampling probability for all operations. ie. the calculated sampling
	// probability will be bound [MinSamplingProbability, 1.0]
	MinSamplingProbability float64

	// LowerBoundTracesPerSecond determines the lower bound number of traces that are sampled per second.
	// For example, if the value is 0.01666666666 (one every minute), then the sampling processor will do
	// its best to sample at least one trace a minute for an operation. This is useful for a low QPS operation
	// that is never sampled by the probabilistic sampler and depends on some time based element.
	LowerBoundTracesPerSecond float64

	// LeaderLeaseRefreshInterval is the duration to sleep if this processor is elected leader before
	// attempting to renew the lease on the leader lock. NB. This should be less than FollowerLeaseRefreshInterval
	// to reduce lock thrashing.
	LeaderLeaseRefreshInterval time.Duration

	// FollowerLeaseRefreshInterval is the duration to sleep if this processor is a follower
	// (ie. failed to gain the leader lock).
	FollowerLeaseRefreshInterval time.Duration
}

// AddFlags adds flags for Options
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Float64(targetQPS, defaultTargetQPS,
		"The target number of sampled traces for all operations.",
	)
	flagSet.Float64(equivalenceThreshold, defaultEquivalenceThreshold,
		"The acceptable amount of deviation for the operation QPS from the `targetQPS`. Increase this to reduce the amount of fluctuation in the probability calculation.",
	)
	flagSet.Int(lookbackQPSCount, defaultLookbackQPSCount,
		"This determines how many previous operation QPS are used in calculating the weighted QPS, ie. if LookbackQPSCount is 1, the only the most recent QPS will be used in calculating the weighted QPS.",
	)
	flagSet.Duration(calculationInterval, defaultCalculationInterval,
		"How often new sampling probabilities are calculated. Recommended to be greater than the polling interval of your clients.",
	)
	flagSet.Duration(lookbackInternval, defaultLookbackInterval,
		"Amount of historical data to look at when calculating new sampling probabilities.",
	)
	flagSet.Duration(delay, defaultDelay,
		"Determines how far back the most recent state is. Use this if you want to add some buffer time for the aggregation to finish.",
	)
	flagSet.Float64(defaultSamplingProbability, defaultDefaultSamplingProbability,
		"The initial sampling probability for all new operations.",
	)
	flagSet.Float64(minSamplingProbability, defaultMinSamplingProbability,
		"The minimum sampling probability for all operations.",
	)
	flagSet.Float64(lowerBoundTracesPerSecond, defaultLowerBoundTracesPerSecond,
		"Determines the lower bound number of traces that are sampled per second.",
	)
	flagSet.Duration(leaderLeaseRefreshInterval, defaultLeaderLeaseRefreshInterval,
		"The duration to sleep if this processor is elected leader before attempting to renew the lease on the leader lock. This should be less than follower-lease-refresh-interval to reduce lock thrashing.",
	)
	flagSet.Duration(followerLeaseRefreshInterval, defaultFollowerLeaseRefreshInterval,
		"The duration to sleep if this processor is a follower.",
	)
}

// InitFromViper initializes Options with properties from viper
func (opts *Options) InitFromViper(v *viper.Viper) *Options {
	opts.TargetQPS = v.GetFloat64(targetQPS)
	opts.QPSEquivalenceThreshold = v.GetFloat64(equivalenceThreshold)
	opts.LookbackQPSCount = v.GetInt(lookbackQPSCount)
	opts.CalculationInterval = v.GetDuration(calculationInterval)
	opts.LookbackInterval = v.GetDuration(lookbackInternval)
	opts.Delay = v.GetDuration(delay)
	opts.DefaultSamplingProbability = v.GetFloat64(defaultSamplingProbability)
	opts.MinSamplingProbability = v.GetFloat64(minSamplingProbability)
	opts.LowerBoundTracesPerSecond = v.GetFloat64(lowerBoundTracesPerSecond)
	opts.LeaderLeaseRefreshInterval = v.GetDuration(leaderLeaseRefreshInterval)
	opts.FollowerLeaseRefreshInterval = v.GetDuration(followerLeaseRefreshInterval)
	return opts
}
