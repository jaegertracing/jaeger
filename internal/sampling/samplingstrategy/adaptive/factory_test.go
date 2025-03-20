// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	smocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/distributedlock"
	lmocks "github.com/jaegertracing/jaeger/pkg/distributedlock/mocks"
)

var (
	_ samplingstrategy.Factory = new(Factory)
	_ storage.Configurable     = new(Factory)
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

	assert.InDelta(t, 5.0, f.options.TargetSamplesPerSecond, 0.01)
	assert.InDelta(t, 0.25, f.options.DeltaTolerance, 1e-3)
	assert.Equal(t, int(2), f.options.BucketsForCalculation)
	assert.Equal(t, time.Minute*15, f.options.CalculationInterval)
	assert.Equal(t, int(3), f.options.AggregationBuckets)
	assert.Equal(t, time.Minute*3, f.options.Delay)
	assert.InDelta(t, 0.02, f.options.InitialSamplingProbability, 1e-3)
	assert.InDelta(t, 0.01, f.options.MinSamplingProbability, 1e-3)
	assert.InDelta(t, 1.0, f.options.MinSamplesPerSecond, 0.01)
	assert.Equal(t, time.Second, f.options.LeaderLeaseRefreshInterval)
	assert.Equal(t, time.Second*2, f.options.FollowerLeaseRefreshInterval)

	require.NoError(t, f.Initialize(api.NullFactory, &mockSamplingStoreFactory{}, zap.NewNop()))
	provider, aggregator, err := f.CreateStrategyProvider()
	require.NoError(t, err)
	require.NoError(t, provider.Close())
	require.NoError(t, aggregator.Close())
	require.NoError(t, f.Close())
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

		require.NoError(t, f.Initialize(api.NullFactory, &mockSamplingStoreFactory{}, zap.NewNop()))
		_, _, err := f.CreateStrategyProvider()
		require.Error(t, err)
		require.NoError(t, f.Close())
	}
}

func TestSamplingStoreFactoryFails(t *testing.T) {
	f := NewFactory()

	// nil fails
	require.Error(t, f.Initialize(api.NullFactory, nil, zap.NewNop()))

	// fail if lock fails
	require.Error(t, f.Initialize(api.NullFactory, &mockSamplingStoreFactory{lockFailsWith: errors.New("fail")}, zap.NewNop()))

	// fail if store fails
	require.Error(t, f.Initialize(api.NullFactory, &mockSamplingStoreFactory{storeFailsWith: errors.New("fail")}, zap.NewNop()))
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

func (m *mockSamplingStoreFactory) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	if m.storeFailsWith != nil {
		return nil, m.storeFailsWith
	}

	mockStorage := &smocks.Store{}
	mockStorage.On("GetLatestProbabilities").Return(make(model.ServiceOperationProbabilities), nil)
	mockStorage.On("GetThroughput", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).
		Return([]*model.Throughput{}, nil)

	return mockStorage, nil
}
