// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestOptionsWithFlags(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--sampling.target-samples-per-second=2.0",
		"--sampling.delta-tolerance=0.6",
		"--sampling.buckets-for-calculation=2",
		"--sampling.calculation-interval=2m0s",
		"--sampling.aggregation-buckets=20",
		"--sampling.delay=6m0s",
		"--sampling.initial-sampling-probability=0.002",
		"--sampling.min-sampling-probability=1e-4",
		"--sampling.min-samples-per-second=0.016666666666666666",
		"--sampling.leader-lease-refresh-interval=5s",
		"--sampling.follower-lease-refresh-interval=1m0s",
	})
	opts := &Options{}

	opts.InitFromViper(v)

	assert.InDelta(t, 2.0, opts.Sampling.TargetRate, 0.01)
	assert.InDelta(t, 0.6, opts.Calculation.DeltaTolerance, 0.01)
	assert.Equal(t, 2, opts.Calculation.Buckets)
	assert.Equal(t, time.Duration(120000000000), opts.Calculation.Interval)
	assert.Equal(t, 20, opts.Calculation.AggregationBuckets)
	assert.Equal(t, time.Duration(360000000000), opts.Calculation.Delay)
	assert.InDelta(t, 0.002, opts.Sampling.InitialProbability, 1e-4)
	assert.InDelta(t, 1e-4, opts.Sampling.MinProbability, 1e-5)
	assert.InDelta(t, 0.016666666666666666, opts.Sampling.MinRate, 1e-3)
	assert.Equal(t, time.Duration(5000000000), opts.LeaderLeaseRefreshInterval)
	assert.Equal(t, time.Duration(60000000000), opts.FollowerLeaseRefreshInterval)
}

func TestDefaultOptions(t *testing.T) {
	options := DefaultOptions()
	assert.InDelta(t, float64(defaultTargetSamplesPerSecond), options.Sampling.TargetRate, 1e-4)
	assert.InDelta(t, defaultDeltaTolerance, options.Calculation.DeltaTolerance, 1e-3)
	assert.Equal(t, defaultBucketsForCalculation, options.Calculation.Buckets)
	assert.Equal(t, defaultCalculationInterval, options.Calculation.Interval)
	assert.Equal(t, defaultAggregationBuckets, options.Calculation.AggregationBuckets)
	assert.Equal(t, defaultDelay, options.Calculation.Delay)
	assert.InDelta(t, defaultInitialSamplingProbability, options.Sampling.InitialProbability, 1e-4)
	assert.InDelta(t, defaultMinSamplingProbability, options.Sampling.MinProbability, 1e-4)
	assert.InDelta(t, defaultMinSamplesPerSecond, options.Sampling.MinRate, 1e-4)
	assert.Equal(t, defaultLeaderLeaseRefreshInterval, options.LeaderLeaseRefreshInterval)
	assert.Equal(t, defaultFollowerLeaseRefreshInterval, options.FollowerLeaseRefreshInterval)
}
