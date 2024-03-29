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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	ss "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	lmocks "github.com/jaegertracing/jaeger/pkg/distributedlock/mocks"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	smocks "github.com/jaegertracing/jaeger/storage/samplingstore/mocks"
)

var (
	_ ss.Factory          = new(Factory)
	_ plugin.Configurable = new(Factory)
)

func TestFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{
		"--sampling.target-samples-per-second=5",
		"--sampling.delta-tolerance=0.25",
		"--sampling.buckets-for-calculation=2",
		"--sampling.calculation-interval=15m",
		"--sampling.aggregation-buckets=3",
		"--sampling.delay=3m",
		"--sampling.initial-sampling-probability=0.02",
		"--sampling.min-sampling-probability=0.01",
		"--sampling.min-samples-per-second=1",
		"--sampling.leader-lease-refresh-interval=1s",
		"--sampling.follower-lease-refresh-interval=2s",
	})

	f.InitFromViper(v, zap.NewNop())

	assert.Equal(t, 5.0, f.options.TargetSamplesPerSecond)
	assert.Equal(t, 0.25, f.options.DeltaTolerance)
	assert.Equal(t, int(2), f.options.BucketsForCalculation)
	assert.Equal(t, time.Minute*15, f.options.CalculationInterval)
	assert.Equal(t, int(3), f.options.AggregationBuckets)
	assert.Equal(t, time.Minute*3, f.options.Delay)
	assert.Equal(t, 0.02, f.options.InitialSamplingProbability)
	assert.Equal(t, 0.01, f.options.MinSamplingProbability)
	assert.Equal(t, 1.0, f.options.MinSamplesPerSecond)
	assert.Equal(t, time.Second, f.options.LeaderLeaseRefreshInterval)
	assert.Equal(t, time.Second*2, f.options.FollowerLeaseRefreshInterval)

	require.NoError(t, f.Initialize(metrics.NullFactory, &mockSamplingStoreFactory{}, zap.NewNop()))
	store, aggregator, err := f.CreateStrategyStore()
	require.NoError(t, err)
	require.NoError(t, store.Close())
	require.NoError(t, aggregator.Close())
}

func TestBadConfigFail(t *testing.T) {
	tests := []string{
		"--sampling.aggregation-buckets=0",
		"--sampling.calculation-interval=0",
		"--sampling.buckets-for-calculation=0",
	}

	for _, tc := range tests {
		f := NewFactory()
		v, command := config.Viperize(f.AddFlags)
		command.ParseFlags([]string{
			tc,
		})

		f.InitFromViper(v, zap.NewNop())

		require.NoError(t, f.Initialize(metrics.NullFactory, &mockSamplingStoreFactory{}, zap.NewNop()))
		_, _, err := f.CreateStrategyStore()
		require.Error(t, err)
	}
}

func TestSamplingStoreFactoryFails(t *testing.T) {
	f := NewFactory()

	// nil fails
	require.Error(t, f.Initialize(metrics.NullFactory, nil, zap.NewNop()))

	// fail if lock fails
	require.Error(t, f.Initialize(metrics.NullFactory, &mockSamplingStoreFactory{lockFailsWith: errors.New("fail")}, zap.NewNop()))

	// fail if store fails
	require.Error(t, f.Initialize(metrics.NullFactory, &mockSamplingStoreFactory{storeFailsWith: errors.New("fail")}, zap.NewNop()))
}

type mockSamplingStoreFactory struct {
	lockFailsWith  error
	storeFailsWith error
}

func (m *mockSamplingStoreFactory) CreateLock() (distributedlock.Lock, error) {
	if m.lockFailsWith != nil {
		return nil, m.lockFailsWith
	}

	mockLock := &lmocks.Lock{}
	mockLock.On("Acquire", mock.Anything, mock.Anything).Return(true, nil)

	return mockLock, nil
}

func (m *mockSamplingStoreFactory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	if m.storeFailsWith != nil {
		return nil, m.storeFailsWith
	}

	mockStorage := &smocks.Store{}
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)
	mockStorage.On("GetThroughput", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return([]*model.Throughput{}, nil)

	return mockStorage, nil
}
