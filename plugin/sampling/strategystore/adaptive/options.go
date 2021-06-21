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
	targetSamplesPerSecond       = "sampling.target-samples-per-second"
	deltaTolerance               = "sampling.delta-tolerance"
	bucketsForCalculation        = "sampling.buckets-for-calculation"
	calculationInterval          = "sampling.calculation-interval"
	aggregationBuckets           = "sampling.aggregation-buckets"
	delay                        = "sampling.delay"
	initialSamplingProbability   = "sampling.initial-sampling-probability"
	minSamplingProbability       = "sampling.min-sampling-probability"
	minSamplesPerSecond          = "sampling.min-samples-per-second"
	leaderLeaseRefreshInterval   = "sampling.leader-lease-refresh-interval"
	followerLeaseRefreshInterval = "sampling.follower-lease-refresh-interval"

	defaultTargetSamplesPerSecond       = 1
	defaultDeltaTolerance               = 0.3
	defaultBucketsForCalculation        = 1
	defaultCalculationInterval          = time.Minute
	defaultAggregationBuckets           = 10
	defaultDelay                        = time.Minute * 2
	defaultInitialSamplingProbability   = 0.001
	defaultMinSamplingProbability       = 1e-5                                   // one in 100k requests
	defaultMinSamplesPerSecond          = 1.0 / float64(time.Minute/time.Second) // once every 1 minute
	defaultLeaderLeaseRefreshInterval   = 5 * time.Second
	defaultFollowerLeaseRefreshInterval = 60 * time.Second
)

// Options holds configuration for the adaptive sampling strategy store.
// The abbreviation SPS refers to "samples-per-second", which is the target
// of the optimization/control implemented by the adaptive sampling.
type Options struct {
	// TargetSamplesPerSecond is the global target rate of samples per operation.
	// TODO implement manual overrides per service/operation.
	TargetSamplesPerSecond float64

	// DeltaTolerance is the acceptable amount of deviation between the observed and the desired (target)
	// throughput for an operation, expressed as a ratio. For example, the value of 0.3 (30% deviation)
	// means that if abs((actual-expected) / expected) < 0.3, then the actual sampling rate is "close enough"
	// and the system does not need to send an updated sampling probability (the "control signal" u(t)
	// in the PID Controller terminology) to the sampler in the application.
	//
	// Increase this to reduce the amount of fluctuation in the calculated probabilities.
	DeltaTolerance float64

	// CalculationInterval determines how often new probabilities are calculated. E.g. if it is 1 minute,
	// new sampling probabilities are calculated once a minute and each bucket will contain 1 minute worth
	// of aggregated throughput data.
	CalculationInterval time.Duration

	// AggregationBuckets is the total number of aggregated throughput buckets kept in memory, ie. if
	// the CalculationInterval is 1 minute (each bucket contains 1 minute of thoughput data) and the
	// AggregationBuckets is 3, the adaptive sampling processor will keep at most 3 buckets in memory for
	// all operations.
	// TODO(wjang): Expand on why this is needed when BucketsForCalculation seems to suffice.
	AggregationBuckets int

	// BucketsForCalculation determines how many previous buckets used in calculating the weighted QPS,
	// ie. if BucketsForCalculation is 1, only the most recent bucket will be used in calculating the weighted QPS.
	BucketsForCalculation int

	// Delay is the amount of time to delay probability generation by, ie. if the CalculationInterval
	// is 1 minute, the number of buckets is 10, and the delay is 2 minutes, then at one time
	// we'll have [now()-12m,now()-2m] range of throughput data in memory to base the calculations
	// off of. This delay is necessary to counteract the rate at which the jaeger clients poll for
	// the latest sampling probabilities. The default client poll rate is 1 minute, which means that
	// during any 1 minute interval, the clients will be fetching new probabilities in a uniformly
	// distributed manner throughout the 1 minute window. By setting the delay to 2 minutes, we can
	// guarantee that all clients can use the latest calculated probabilities for at least 1 minute.
	Delay time.Duration

	// InitialSamplingProbability is the initial sampling probability for all new operations.
	InitialSamplingProbability float64

	// MinSamplingProbability is the minimum sampling probability for all operations. ie. the calculated sampling
	// probability will be in the range [MinSamplingProbability, 1.0].
	MinSamplingProbability float64

	// MinSamplesPerSecond determines the min number of traces that are sampled per second.
	// For example, if the value is 0.01666666666 (one every minute), then the sampling processor will do
	// its best to sample at least one trace a minute for an operation. This is useful for low QPS operations
	// that may never be sampled by the probabilistic sampler.
	MinSamplesPerSecond float64

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
	flagSet.Float64(targetSamplesPerSecond, defaultTargetSamplesPerSecond,
		"The the global target rate of samples per operation.",
	)
	flagSet.Float64(deltaTolerance, defaultDeltaTolerance,
		"The acceptable amount of deviation between the observed samples-per-second and the desired (target) samples-per-second, expressed as a ratio.",
	)
	flagSet.Int(bucketsForCalculation, defaultBucketsForCalculation,
		"This determines how much of the previous data is used in calculating the weighted QPS, ie. if BucketsForCalculation is 1, only the most recent data will be used in calculating the weighted QPS.",
	)
	flagSet.Duration(calculationInterval, defaultCalculationInterval,
		"How often new sampling probabilities are calculated. Recommended to be greater than the polling interval of your clients.",
	)
	flagSet.Int(aggregationBuckets, defaultAggregationBuckets,
		"Amount of historical data to keep in memory.",
	)
	flagSet.Duration(delay, defaultDelay,
		"Determines how far back the most recent state is. Use this if you want to add some buffer time for the aggregation to finish.",
	)
	flagSet.Float64(initialSamplingProbability, defaultInitialSamplingProbability,
		"The initial sampling probability for all new operations.",
	)
	flagSet.Float64(minSamplingProbability, defaultMinSamplingProbability,
		"The minimum sampling probability for all operations.",
	)
	flagSet.Float64(minSamplesPerSecond, defaultMinSamplesPerSecond,
		"The minimum number of traces that are sampled per second.",
	)
	flagSet.Duration(leaderLeaseRefreshInterval, defaultLeaderLeaseRefreshInterval,
		"The duration to sleep if this processor is elected leader before attempting to renew the lease on the leader lock. This should be less than follower-lease-refresh-interval to reduce lock thrashing.",
	)
	flagSet.Duration(followerLeaseRefreshInterval, defaultFollowerLeaseRefreshInterval,
		"The duration to sleep if this processor is a follower.",
	)
}

// InitFromViper initializes Options with properties from viper
func (opts Options) InitFromViper(v *viper.Viper) Options {
	opts.TargetSamplesPerSecond = v.GetFloat64(targetSamplesPerSecond)
	opts.DeltaTolerance = v.GetFloat64(deltaTolerance)
	opts.BucketsForCalculation = v.GetInt(bucketsForCalculation)
	opts.CalculationInterval = v.GetDuration(calculationInterval)
	opts.AggregationBuckets = v.GetInt(aggregationBuckets)
	opts.Delay = v.GetDuration(delay)
	opts.InitialSamplingProbability = v.GetFloat64(initialSamplingProbability)
	opts.MinSamplingProbability = v.GetFloat64(minSamplingProbability)
	opts.MinSamplesPerSecond = v.GetFloat64(minSamplesPerSecond)
	opts.LeaderLeaseRefreshInterval = v.GetDuration(leaderLeaseRefreshInterval)
	opts.FollowerLeaseRefreshInterval = v.GetDuration(followerLeaseRefreshInterval)
	return opts
}
