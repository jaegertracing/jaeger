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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestFlagDefaults(t *testing.T) {
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{})
	opts := Options{}

	opts.InitFromViper(v)
	assert.Equal(t, 1.0, opts.TargetSamplesPerSecond)
	assert.Equal(t, 0.3, opts.DeltaTolerance)
	assert.Equal(t, 1, opts.BucketsForCalculation)
	assert.Equal(t, time.Minute, opts.CalculationInterval)
	assert.Equal(t, 10, opts.AggregationBuckets)
	assert.Equal(t, time.Minute*2, opts.Delay)
	assert.Equal(t, 0.001, opts.InitialSamplingProbability)
	assert.Equal(t, 1e-5, opts.MinSamplingProbability)
	assert.Equal(t, (1.0 / float64(time.Minute/time.Second)), opts.MinSamplesPerSecond)
	assert.Equal(t, 5*time.Second, opts.LeaderLeaseRefreshInterval)
	assert.Equal(t, 60*time.Second, opts.FollowerLeaseRefreshInterval)
}

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

	assert.Equal(t, 2.0, opts.TargetSamplesPerSecond)
	assert.Equal(t, 0.6, opts.DeltaTolerance)
	assert.Equal(t, 2, opts.BucketsForCalculation)
	assert.Equal(t, time.Duration(120000000000), opts.CalculationInterval)
	assert.Equal(t, 20, opts.AggregationBuckets)
	assert.Equal(t, time.Duration(360000000000), opts.Delay)
	assert.Equal(t, 0.002, opts.InitialSamplingProbability)
	assert.Equal(t, 1e-4, opts.MinSamplingProbability)
	assert.Equal(t, 0.016666666666666666, opts.MinSamplesPerSecond)
	assert.Equal(t, time.Duration(5000000000), opts.LeaderLeaseRefreshInterval)
	assert.Equal(t, time.Duration(60000000000), opts.FollowerLeaseRefreshInterval)
}
