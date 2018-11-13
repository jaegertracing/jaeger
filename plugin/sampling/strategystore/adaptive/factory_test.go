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
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	ss "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin"
)

var _ ss.Factory = new(Factory)
var _ plugin.Configurable = new(Factory)

func TestFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{
		"--sampling.target-qps=5",
		"--sampling.equivalence-threshold=0.25",
		"--sampling.lookback-qps-count=2",
		"--sampling.calculation-interval=15m",
		"--sampling.lookback-interval=3m",
		"--sampling.delay=3m",
		"--sampling.default-sampling-probability=0.02",
		"--sampling.min-sampling-probability=0.01",
		"--sampling.lower-bound-traces-per-second=1",
		"--sampling.leader-lease-refresh-interval=1s",
		"--sampling.follower-lease-refresh-interval=2s",
	})

	f.InitFromViper(v)

	assert.Equal(t, 5.0, f.options.TargetQPS)
	assert.Equal(t, 0.25, f.options.QPSEquivalenceThreshold)
	assert.Equal(t, int(2), f.options.LookbackQPSCount)
	assert.Equal(t, time.Minute*15, f.options.CalculationInterval)
	assert.Equal(t, time.Minute*3, f.options.LookbackInterval)
	assert.Equal(t, time.Minute*3, f.options.Delay)
	assert.Equal(t, 0.02, f.options.DefaultSamplingProbability)
	assert.Equal(t, 0.01, f.options.MinSamplingProbability)
	assert.Equal(t, 1.0, f.options.LowerBoundTracesPerSecond)
	assert.Equal(t, time.Second, f.options.LeaderLeaseRefreshInterval)
	assert.Equal(t, time.Second*2, f.options.FollowerLeaseRefreshInterval)

	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	_, err := f.CreateStrategyStore()
	assert.NoError(t, err)
}
