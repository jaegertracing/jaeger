// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
	TargetSamplesPerSecond float64 `mapstructure:"target_samples_per_second"`

	// DeltaTolerance is the acceptable amount of deviation between the observed and the desired (target)
	// throughput for an operation, expressed as a ratio. For example, the value of 0.3 (30% deviation)
	// means that if abs((actual-expected) / expected) < 0.3, then the actual sampling rate is "close enough"
	// and the system does not need to send an updated sampling probability (the "control signal" u(t)
	// in the PID Controller terminology) to the sampler in the application.
	//
	// Increase this to reduce the amount of fluctuation in the calculated probabilities.
	DeltaTolerance float64 `mapstructure:"delta_tolerance"`

	// CalculationInterval determines how often new probabilities are calculated. E.g. if it is 1 minute,
	// new sampling probabilities are calculated once a minute and each bucket will contain 1 minute worth
	// of aggregated throughput data.
	CalculationInterval time.Duration `mapstructure:"calculation_interval"`

	// AggregationBuckets is the total number of aggregated throughput buckets kept in memory, ie. if
	// the CalculationInterval is 1 minute (each bucket contains 1 minute of thoughput data) and the
	// AggregationBuckets is 3, the adaptive sampling processor will keep at most 3 buckets in memory for
	// all operations.
	// TODO(wjang): Expand on why this is needed when BucketsForCalculation seems to suffice.
	AggregationBuckets int `mapstructure:"aggregation_buckets"`

	// BucketsForCalculation determines how many previous buckets used in calculating the weighted QPS,
	// ie. if BucketsForCalculation is 1, only the most recent bucket will be used in calculating the weighted QPS.
	BucketsForCalculation int `mapstructure:"calculation_buckets"`

	// Delay is the amount of time to delay probability generation by, ie. if the CalculationInterval
	// is 1 minute, the number of buckets is 10, and the delay is 2 minutes, then at one time
	// we'll have [now()-12m,now()-2m] range of throughput data in memory to base the calculations
	// off of. This delay is necessary to counteract the rate at which the jaeger clients poll for
	// the latest sampling probabilities. The default client poll rate is 1 minute, which means that
	// during any 1 minute interval, the clients will be fetching new probabilities in a uniformly
	// distributed manner throughout the 1 minute window. By setting the delay to 2 minutes, we can
	// guarantee that all clients can use the latest calculated probabilities for at least 1 minute.
	Delay time.Duration `mapstructure:"calculation_delay"`

	// InitialSamplingProbability is the initial sampling probability for all new operations.
	InitialSamplingProbability float64 `mapstructure:"initial_sampling_probability"`

	// MinSamplingProbability is the minimum sampling probability for all operations. ie. the calculated sampling
	// probability will be in the range [MinSamplingProbability, 1.0].
	MinSamplingProbability float64 `mapstructure:"min_sampling_probability"`

	// MinSamplesPerSecond determines the min number of traces that are sampled per second.
	// For example, if the value is 0.01666666666 (one every minute), then the sampling processor will do
	// its best to sample at least one trace a minute for an operation. This is useful for low QPS operations
	// that may never be sampled by the probabilistic sampler.
	MinSamplesPerSecond float64 `mapstructure:"min_samples_per_second"`

	// LeaderLeaseRefreshInterval is the duration to sleep if this processor is elected leader before
	// attempting to renew the lease on the leader lock. NB. This should be less than FollowerLeaseRefreshInterval
	// to reduce lock thrashing.
	LeaderLeaseRefreshInterval time.Duration `mapstructure:"leader_lease_refresh_interval"`

	// FollowerLeaseRefreshInterval is the duration to sleep if this processor is a follower
	// (ie. failed to gain the leader lock).
	FollowerLeaseRefreshInterval time.Duration `mapstructure:"follower_lease_refresh_interval"`

	// Use at your own risk!  When this setting is enabled, the engine will not attempt
	// to infer the actual sampling probability used in the SDKs and may cause a spike
	// of trace volume under the conditions explained below.
	//
	// The original adaptive sampling logic was built to work with legacy Jaeger SDK
	// which used to report via span tag when the probabilistic sampled was used and
	// with which probability value. The sampler implementation in the OpenTelemetry
	// SDKs do not include such span tags, which makes it impossible for the engine
	// to verify if the adaptive sampling rates are being respected / used by the sampler.
	// However, this validation is not critical to the engine's operation, as it was
	// done as a protection measure against a situation when a non-adaptive sampler
	// is used in the SDK with a very low probability, and the engine keeps trying
	// to increase this probability and not seeing an expected change in the trace
	// volume (aka throughput), which will eventually result in the calculated
	// probability reaching 100%. This could present a danger if the SDK is then
	// switched to respect adaptive sampling rate since it will drastically increase
	// the volume of traces sampled and the engine will take a few minutes to react
	// to that.
	IgnoreSamplerTags bool
}

func DefaultOptions() Options {
	return Options{
		TargetSamplesPerSecond:       defaultTargetSamplesPerSecond,
		DeltaTolerance:               defaultDeltaTolerance,
		BucketsForCalculation:        defaultBucketsForCalculation,
		CalculationInterval:          defaultCalculationInterval,
		AggregationBuckets:           defaultAggregationBuckets,
		Delay:                        defaultDelay,
		InitialSamplingProbability:   defaultInitialSamplingProbability,
		MinSamplingProbability:       defaultMinSamplingProbability,
		MinSamplesPerSecond:          defaultMinSamplesPerSecond,
		LeaderLeaseRefreshInterval:   defaultLeaderLeaseRefreshInterval,
		FollowerLeaseRefreshInterval: defaultFollowerLeaseRefreshInterval,
	}
}

// AddFlags adds flags for Options
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Float64(targetSamplesPerSecond, defaultTargetSamplesPerSecond,
		"The global target rate of samples per operation.",
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
func (opts *Options) InitFromViper(v *viper.Viper) *Options {
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
