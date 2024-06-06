// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import "time"

type Config struct {
	// name of the strategy storage defined in the jaegerstorage extension
	StrategyStore string `mapstructure:"strategy_store"`

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

	// BucketsForCalculation determines how many previous buckets used in calculating the weighted QPS,
	// ie. if BucketsForCalculation is 1, only the most recent bucket will be used in calculating the weighted QPS.
	BucketsForCalculation int `mapstructure:"buckets_for_calculation"`

	// Delay is the amount of time to delay probability generation by, ie. if the CalculationInterval
	// is 1 minute, the number of buckets is 10, and the delay is 2 minutes, then at one time
	// we'll have [now()-12m,now()-2m] range of throughput data in memory to base the calculations
	// off of. This delay is necessary to counteract the rate at which the SDKs poll for
	// the latest sampling probabilities. The default client poll rate is 1 minute, which means that
	// during any 1 minute interval, the clients will be fetching new probabilities in a uniformly
	// distributed manner throughout the 1 minute window. By setting the delay to 2 minutes, we can
	// guarantee that all clients can use the latest calculated probabilities for at least 1 minute.
	Delay time.Duration `mapstructure:"delay"`

	// MinSamplingProbability is the minimum sampling probability for all operations. ie. the calculated sampling
	// probability will be in the range [MinSamplingProbability, 1.0].
	MinSamplingProbability float64 `mapstructure:"min_sampling_probability"`
}
