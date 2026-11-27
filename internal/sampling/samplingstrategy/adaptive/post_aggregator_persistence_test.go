// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	spanmodel "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/distributedlock"
	"github.com/jaegertracing/jaeger/internal/leaderelection"
	epmocks "github.com/jaegertracing/jaeger/internal/leaderelection/mocks"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esclientmocks "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	smocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	essamplestore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore"
	esdbmodel "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type testLockRegistry struct {
	mu    sync.Mutex
	owner string
}

type testParticipantLock struct {
	reg    *testLockRegistry
	id     string
	fenced bool
}

func (l *testParticipantLock) setFenced(fenced bool) {
	l.reg.mu.Lock()
	defer l.reg.mu.Unlock()
	l.fenced = fenced
	if fenced && l.reg.owner == l.id {
		l.reg.owner = ""
	}
}

func (l *testParticipantLock) Acquire(string, time.Duration) (bool, error) {
	l.reg.mu.Lock()
	defer l.reg.mu.Unlock()
	if l.fenced {
		return false, nil
	}
	if l.reg.owner == "" || l.reg.owner == l.id {
		l.reg.owner = l.id
		return true, nil
	}
	return false, nil
}

func (l *testParticipantLock) Forfeit(string) (bool, error) {
	l.reg.mu.Lock()
	defer l.reg.mu.Unlock()
	if l.reg.owner != l.id {
		return false, nil
	}
	l.reg.owner = ""
	return true, nil
}

type writeNotifyStore struct {
	samplingstore.Store
	wrote chan struct{}
}

func (s *writeNotifyStore) InsertProbabilitiesAndQPS(
	hostname string,
	probabilities model.ServiceOperationProbabilities,
	qps model.ServiceOperationQPS,
) error {
	err := s.Store.InsertProbabilitiesAndQPS(hostname, probabilities, qps)
	select {
	case s.wrote <- struct{}{}:
	default:
	}
	return err
}

func makeProbabilisticRootSpan(service, operation string, prob float64) *spanmodel.Span {
	return &spanmodel.Span{
		Process:       &spanmodel.Process{ServiceName: service},
		OperationName: operation,
		Tags: spanmodel.KeyValues{
			spanmodel.String("sampler.type", "probabilistic"),
			spanmodel.String("sampler.param", strconv.FormatFloat(prob, 'g', -1, 64)),
		},
	}
}

func newTestAdaptiveOpts(interval time.Duration, targetSPS, initialProb float64) Options {
	return Options{
		TargetSamplesPerSecond:       targetSPS,
		DeltaTolerance:               0.3,
		CalculationInterval:          interval,
		AggregationBuckets:           3,
		BucketsForCalculation:        1,
		InitialSamplingProbability:   initialProb,
		MinSamplingProbability:       1e-5,
		MinSamplesPerSecond:          1.0 / 60.0,
		LeaderLeaseRefreshInterval:   100 * time.Millisecond,
		FollowerLeaseRefreshInterval: 200 * time.Millisecond,
	}
}

func startTestElection(t *testing.T, lock distributedlock.Lock, opts Options) (*leaderelection.DistributedElectionParticipant, func()) {
	t.Helper()
	ep := leaderelection.NewElectionParticipant(lock, "sampling_store_leader", leaderelection.ElectionParticipantOptions{
		LeaderLeaseRefreshInterval:   opts.LeaderLeaseRefreshInterval,
		FollowerLeaseRefreshInterval: opts.FollowerLeaseRefreshInterval,
		Logger:                       zap.NewNop(),
	})
	require.NoError(t, ep.Start())
	var once sync.Once
	closeFn := func() { once.Do(func() { require.NoError(t, ep.Close()) }) }
	t.Cleanup(closeFn)
	return ep, closeFn
}

func startTestAggregator(t *testing.T, opts Options, ep leaderelection.ElectionParticipant, store samplingstore.Store) (samplingstrategy.Aggregator, func()) {
	t.Helper()
	agg, err := NewAggregator(opts, zap.NewNop(), metrics.NullFactory, ep, store)
	require.NoError(t, err)
	agg.Start()
	var once sync.Once
	closeFn := func() { once.Do(func() { require.NoError(t, agg.Close()) }) }
	t.Cleanup(closeFn)
	return agg, closeFn
}

func openPersistentBadger(t *testing.T, dir string) (*badger.Factory, samplingstore.Store, distributedlock.Lock) {
	t.Helper()
	f := badger.NewFactory()
	f.Config.Ephemeral = false
	f.Config.Directories.Keys = dir
	f.Config.Directories.Values = dir
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	store, err := f.CreateSamplingStore(10)
	require.NoError(t, err)
	lock, err := f.CreateLock()
	require.NoError(t, err)
	return f, store, lock
}

func TestPostAggregatorRestoresPersistedProbabilitiesOnRestart(t *testing.T) {
	dir := t.TempDir()
	opts := newTestAdaptiveOpts(50*time.Millisecond, 20.0, 0.1)
	opts.DeltaTolerance = 0.4
	opts.LeaderLeaseRefreshInterval = 5 * time.Millisecond
	opts.FollowerLeaseRefreshInterval = 50 * time.Millisecond

	// Phase 1: Initial collector converges rate from 0.1 down below 0.01 on persistent disk storage.
	f1, raw1, lock1 := openPersistentBadger(t, dir)
	store1 := &writeNotifyStore{Store: raw1, wrote: make(chan struct{}, 16)}
	ep1, closeEP1 := startTestElection(t, lock1, opts)
	require.Eventually(t, ep1.IsLeader, 2*time.Second, time.Millisecond)

	agg1, closeAgg1 := startTestAggregator(t, opts, ep1, store1)
	for range 200 {
		agg1.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, 0.1))
	}

	var convergedProb float64
	require.Eventually(t, func() bool {
		select {
		case <-store1.wrote:
		default:
		}
		probs, err := store1.GetLatestProbabilities()
		if err != nil {
			return false
		}
		if p, ok := probs["svcA"][http.MethodGet]; ok && p < 0.01 {
			convergedProb = p
			return true
		}
		return false
	}, 2*time.Second, 5*time.Millisecond)

	closeAgg1()
	closeEP1()
	require.NoError(t, f1.Close())

	// Phase 2: Collector restarts against the same Badger directory and sees ~target QPS at convergedProb.
	f2, raw2, lock2 := openPersistentBadger(t, dir)
	defer func() { require.NoError(t, f2.Close()) }()
	store2 := &writeNotifyStore{Store: raw2, wrote: make(chan struct{}, 16)}
	ep2, _ := startTestElection(t, lock2, opts)
	require.Eventually(t, ep2.IsLeader, 2*time.Second, time.Millisecond)

	agg2, _ := startTestAggregator(t, opts, ep2, store2)
	agg2.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, convergedProb))

	select {
	case <-store2.wrote:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PostAggregator write after restart")
	}

	persistedAfterTick, err := store2.GetLatestProbabilities()
	require.NoError(t, err)
	assert.InDelta(t, convergedProb, persistedAfterTick["svcA"][http.MethodGet], 1e-6)
}

func TestPostAggregatorFollowerPromotionPreservesProbabilities(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sharedStore := memory.NewSamplingStore(10)
		reg := &testLockRegistry{}
		opts := newTestAdaptiveOpts(time.Second, 1.0, 0.1)

		ep1, closeEP1 := startTestElection(t, &testParticipantLock{reg: reg, id: "node-1"}, opts)
		synctest.Wait()
		require.True(t, ep1.IsLeader())

		ep2, closeEP2 := startTestElection(t, &testParticipantLock{reg: reg, id: "node-2"}, opts)
		synctest.Wait()
		require.False(t, ep2.IsLeader())

		agg1, closeAgg1 := startTestAggregator(t, opts, ep1, sharedStore)
		agg2, closeAgg2 := startTestAggregator(t, opts, ep2, sharedStore)
		defer func() {
			closeAgg2()
			closeEP2()
			closeEP1()
		}()

		// Leader node-1 converges (svcA, GET) from 0.1 to 0.0001.
		for range 1000 {
			agg1.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, 0.1))
		}
		time.Sleep(time.Second + time.Millisecond)
		synctest.Wait()

		probs, err := sharedStore.GetLatestProbabilities()
		require.NoError(t, err)
		require.InDelta(t, 0.0001, probs["svcA"][http.MethodGet], 1e-9)

		// Advance a follower tick, then hand over leadership cleanly to node-2.
		time.Sleep(time.Second)
		synctest.Wait()
		closeAgg1()
		closeEP1()
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		require.True(t, ep2.IsLeader())

		// New leader sees 1.0 QPS already matching target at 0.0001 and must not reset to 0.1.
		agg2.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, 0.0001))
		time.Sleep(time.Second + time.Millisecond)
		synctest.Wait()

		afterFailover, err := sharedStore.GetLatestProbabilities()
		require.NoError(t, err)
		assert.InDelta(t, 0.0001, afterFailover["svcA"][http.MethodGet], 1e-9)
	})
}

func TestPostAggregatorReElectedLeaderRefreshesStaleProbabilities(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sharedStore := memory.NewSamplingStore(10)
		reg := &testLockRegistry{}
		lock1 := &testParticipantLock{reg: reg, id: "node-1"}
		opts := newTestAdaptiveOpts(time.Second, 1.0, 0.1)

		ep1, closeEP1 := startTestElection(t, lock1, opts)
		synctest.Wait()
		require.True(t, ep1.IsLeader())

		agg1, closeAgg1 := startTestAggregator(t, opts, ep1, sharedStore)
		defer func() {
			closeAgg1()
			closeEP1()
		}()

		// Term 1 (node-1): 2 QPS at 0.1 -> writes 0.05 to storage and node-1 memory.
		for range 2 {
			agg1.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, 0.1))
		}
		time.Sleep(time.Second + time.Millisecond)
		synctest.Wait()

		term1, err := sharedStore.GetLatestProbabilities()
		require.NoError(t, err)
		require.InDelta(t, 0.05, term1["svcA"][http.MethodGet], 1e-9)

		// Sub-interval failback: node-1 loses lock, node-2 writes 0.0005, node-1 regains lock before ticking.
		lock1.setFenced(true)
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		require.False(t, ep1.IsLeader())

		ep2, closeEP2 := startTestElection(t, &testParticipantLock{reg: reg, id: "node-2"}, opts)
		synctest.Wait()
		require.True(t, ep2.IsLeader())
		require.NoError(t, sharedStore.InsertProbabilitiesAndQPS(
			"node-2",
			model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.0005}},
			model.ServiceOperationQPS{"svcA": {http.MethodGet: 1.0}},
		))
		closeEP2()

		lock1.setFenced(false)
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		require.True(t, ep1.IsLeader())

		// Re-elected node-1 sees 5 QPS at 0.0005 and must scale down to 0.0001 rather than revert to stale 0.05.
		for range 5 {
			agg1.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, 0.0005))
		}
		time.Sleep(time.Second + time.Millisecond)
		synctest.Wait()

		afterFailback, err := sharedStore.GetLatestProbabilities()
		require.NoError(t, err)
		assert.InDelta(t, 0.0001, afterFailback["svcA"][http.MethodGet], 1e-9)
	})
}

func TestPostAggregatorSubIntervalFailbackInvalidatesServiceCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sharedStore := memory.NewSamplingStore(10)
		reg := &testLockRegistry{}
		lock1 := &testParticipantLock{reg: reg, id: "node-1"}
		opts := newTestAdaptiveOpts(time.Second, 1.0, 0.1)

		ep1, closeEP1 := startTestElection(t, lock1, opts)
		synctest.Wait()
		require.True(t, ep1.IsLeader())

		agg1, closeAgg1 := startTestAggregator(t, opts, ep1, sharedStore)
		defer func() {
			closeAgg1()
			closeEP1()
		}()

		// Tick 1: converge (svcA, GET) from 0.1 to 0.05.
		for range 2 {
			agg1.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, 0.1))
		}
		time.Sleep(time.Second + time.Millisecond)
		synctest.Wait()

		// Tick 2: observe 2 QPS at 0.05 (non-initial) so serviceCache records UsingAdaptive=true for 0.05 and writes 0.025.
		for range 2 {
			agg1.HandleRootSpan(makeProbabilisticRootSpan("svcA", http.MethodGet, 0.05))
		}
		time.Sleep(time.Second)
		synctest.Wait()

		term1, err := sharedStore.GetLatestProbabilities()
		require.NoError(t, err)
		require.InDelta(t, 0.025, term1["svcA"][http.MethodGet], 1e-9)

		// Sub-interval failback: node-1 loses lock, node-2 persists 0.002, node-1 regains lock before Tick 3.
		lock1.setFenced(true)
		time.Sleep(150 * time.Millisecond)
		synctest.Wait()
		require.False(t, ep1.IsLeader())

		ep2, closeEP2 := startTestElection(t, &testParticipantLock{reg: reg, id: "node-2"}, opts)
		synctest.Wait()
		require.True(t, ep2.IsLeader())
		require.NoError(t, sharedStore.InsertProbabilitiesAndQPS(
			"node-2",
			model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.002}},
			model.ServiceOperationQPS{"svcA": {http.MethodGet: 1.0}},
		))
		closeEP2()

		lock1.setFenced(false)
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		require.True(t, ep1.IsLeader())

		// Tick 3 fires with zero spans in the latest bucket while Tick 2's bucket remains in throughputs[1].
		// Re-elected node-1 must not use its stale pre-failover serviceCache entry to scale node-2's 0.002.
		time.Sleep(650 * time.Millisecond)
		synctest.Wait()

		afterFailback, err := sharedStore.GetLatestProbabilities()
		require.NoError(t, err)
		assert.InDelta(t, 0.002, afterFailback["svcA"][http.MethodGet], 1e-9)
	})
}

func TestPostAggregatorNearTargetPreservesConvergedDirection(t *testing.T) {
	tests := []struct {
		name            string
		initialProb     float64
		convergedStored float64
	}{
		{name: "converged_below_initial", initialProb: 0.1, convergedStored: 0.0001},
		{name: "converged_above_initial", initialProb: 0.001, convergedStored: 0.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := memory.NewSamplingStore(10)
				require.NoError(t, store.InsertProbabilitiesAndQPS(
					"prev-leader",
					model.ServiceOperationProbabilities{"svcA": {http.MethodGet: tt.convergedStored}},
					model.ServiceOperationQPS{"svcA": {http.MethodGet: 1.0}},
				))

				opts := newTestAdaptiveOpts(time.Second, 1.0, tt.initialProb)
				opts.LeaderLeaseRefreshInterval = 50 * time.Millisecond
				ep, closeEP := startTestElection(t, &testParticipantLock{reg: &testLockRegistry{}, id: "leader"}, opts)
				defer closeEP()
				synctest.Wait()
				require.True(t, ep.IsLeader())

				pa, err := newPostAggregator(opts, "leader", store, ep, metrics.NullFactory, zap.NewNop())
				require.NoError(t, err)
				pa.Start()

				time.Sleep(time.Second)
				require.NoError(t, store.InsertThroughput([]*model.Throughput{
					{
						Service:       "svcA",
						Operation:     http.MethodGet,
						Count:         1,
						Probabilities: map[string]struct{}{TruncateFloat(tt.convergedStored): {}},
					},
				}))
				synctest.Wait()
				pa.runCalculation()

				got, err := store.GetLatestProbabilities()
				require.NoError(t, err)
				assert.InDelta(t, tt.convergedStored, got["svcA"][http.MethodGet], 1e-9)
			})
		})
	}
}

func TestPostAggregatorNilProbabilitiesOnStartup(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", mock.Anything, mock.Anything).Return(nil, nil)
	mockStorage.On("GetLatestProbabilities").Return(nil, nil).Once()

	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("IsLeader").Return(false)

	pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "host", mockStorage, mockEP, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	pa.Start()

	pa.mu.RLock()
	probs := pa.probabilities
	pa.mu.RUnlock()
	require.NotNil(t, probs)
	assert.Empty(t, probs)
	mockStorage.AssertExpectations(t)
}

func TestPostAggregatorGetLatestProbabilitiesErrorRetriesOnNextTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger, logBuffer := testutils.NewLogger()
		mockStorage := &smocks.Store{}
		mockStorage.On("GetThroughput", mock.Anything, mock.Anything).Return([]*model.Throughput{
			{
				Service:       "svcA",
				Operation:     http.MethodGet,
				Count:         1,
				Probabilities: map[string]struct{}{"0.000100": {}},
			},
		}, nil)
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities(nil), errors.New("storage down")).Once()
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.0001}}, nil).Once()
		mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.Anything, mock.Anything).Return(nil)

		mockEP := &epmocks.ElectionParticipant{}
		mockEP.On("IsLeader").Return(false).Once()
		mockEP.On("IsLeader").Return(true)

		pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "host", mockStorage, mockEP, metrics.NullFactory, logger)
		require.NoError(t, err)

		pa.Start()
		match, errMsg := testutils.LogMatcher(1, "failed to initialize probabilities", logBuffer.Lines())
		assert.True(t, match, errMsg)

		// On the next tick as newly elected leader, it retries GetLatestProbabilities() and restores 0.0001.
		time.Sleep(time.Second)
		synctest.Wait()
		pa.runCalculation()

		pa.mu.RLock()
		got := pa.probabilities["svcA"][http.MethodGet]
		pa.mu.RUnlock()
		assert.InDelta(t, 0.0001, got, 1e-9)
		mockStorage.AssertExpectations(t)
	})
}

func TestPostAggregatorSteadyLeaderBoundedStorageReads(t *testing.T) {
	mockStorage := &smocks.Store{}
	mockStorage.On("GetThroughput", mock.Anything, mock.Anything).Return([]*model.Throughput{
		{Service: "svcA", Operation: http.MethodGet, Count: 10},
		{Service: "svcA", Operation: http.MethodPost, Count: 20},
		{Service: "svcB", Operation: http.MethodGet, Count: 30},
	}, nil)
	mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.02}}, nil)
	mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.Anything, mock.Anything).Return(nil)

	mockEP := &epmocks.ElectionParticipant{}
	mockEP.On("IsLeader").Return(true)

	pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "host", mockStorage, mockEP, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	pa.Start()

	// Three ticks across multiple services/operations must perform O(1) read per tick (1 on Start + 3 on ticks),
	// never per-service or per-operation reads inside calculateProbability().
	pa.runCalculation()
	pa.runCalculation()
	pa.runCalculation()

	mockStorage.AssertNumberOfCalls(t, "GetLatestProbabilities", 4)
}

func TestPostAggregatorStartupReadFailureDefersUntilRetrySucceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger, logBuffer := testutils.NewLogger()
		mockStorage := &smocks.Store{}
		mockStorage.On("GetThroughput", mock.Anything, mock.Anything).Return([]*model.Throughput{
			{
				Service:       "svcA",
				Operation:     http.MethodGet,
				Count:         1,
				Probabilities: map[string]struct{}{"0.000100": {}},
			},
		}, nil)
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities(nil), errors.New("storage down")).Twice()
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.0001}}, nil).Once()

		var saved model.ServiceOperationProbabilities
		mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				saved = args.Get(1).(model.ServiceOperationProbabilities)
			}).
			Return(nil).
			Once()

		mockEP := &epmocks.ElectionParticipant{}
		mockEP.On("IsLeader").Return(true)

		pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "host", mockStorage, mockEP, metrics.NullFactory, logger)
		require.NoError(t, err)

		// Initial Start() read fails; leader must defer calculation and publication on tick 1 while read fails.
		pa.Start()
		time.Sleep(time.Second)
		synctest.Wait()
		pa.runCalculation()

		match, errMsg := testutils.LogMatcher(2, "failed to initialize probabilities", logBuffer.Lines())
		assert.True(t, match, errMsg)
		mockStorage.AssertNotCalled(t, "InsertProbabilitiesAndQPS", mock.Anything, mock.Anything, mock.Anything)
		pa.mu.RLock()
		assert.Empty(t, pa.probabilities)
		pa.mu.RUnlock()

		// Retry tick succeeds: restores 0.0001 and computes/saves 0.0001 rather than InitialSamplingProbability (0.1).
		time.Sleep(time.Second)
		synctest.Wait()
		pa.runCalculation()

		require.InDelta(t, 0.0001, saved["svcA"][http.MethodGet], 1e-9)
		pa.mu.RLock()
		got := pa.probabilities["svcA"][http.MethodGet]
		pa.mu.RUnlock()
		assert.InDelta(t, 0.0001, got, 1e-9)
		mockStorage.AssertExpectations(t)
	})
}

func TestPostAggregatorPromotionRefreshFailureDefersUntilRetrySucceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockStorage := &smocks.Store{}
		mockStorage.On("GetThroughput", mock.Anything, mock.Anything).Return([]*model.Throughput{
			{
				Service:       "svcA",
				Operation:     http.MethodGet,
				Count:         1,
				Probabilities: map[string]struct{}{"0.000100": {}},
			},
		}, nil)
		// Start() as follower reads stale 0.05; promotion refresh fails once, then retry succeeds with 0.0001.
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.05}}, nil).Once()
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities(nil), errors.New("storage down")).Once()
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.0001}}, nil).Once()

		var saved model.ServiceOperationProbabilities
		mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				saved = args.Get(1).(model.ServiceOperationProbabilities)
			}).
			Return(nil).
			Once()

		mockEP := &epmocks.ElectionParticipant{}
		mockEP.On("IsLeader").Return(false).Once()
		mockEP.On("IsLeader").Return(true)

		pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "host", mockStorage, mockEP, metrics.NullFactory, zap.NewNop())
		require.NoError(t, err)
		pa.Start()

		// First leader tick after promotion fails refresh: must preserve 0.05 in memory and publish nothing.
		time.Sleep(time.Second)
		synctest.Wait()
		pa.runCalculation()

		mockStorage.AssertNotCalled(t, "InsertProbabilitiesAndQPS", mock.Anything, mock.Anything, mock.Anything)
		pa.mu.RLock()
		preserved := pa.probabilities["svcA"][http.MethodGet]
		pa.mu.RUnlock()
		assert.InDelta(t, 0.05, preserved, 1e-9)

		// Second tick retry succeeds with 0.0001 and publishes 0.0001 rather than stale 0.05.
		time.Sleep(time.Second)
		synctest.Wait()
		pa.runCalculation()

		require.InDelta(t, 0.0001, saved["svcA"][http.MethodGet], 1e-9)
		mockStorage.AssertExpectations(t)
	})
}

func TestPostAggregatorLeadershipChangeBeforeCheckUsesPostPromotionRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sharedStore := memory.NewSamplingStore(10)
		require.NoError(t, sharedStore.InsertProbabilitiesAndQPS(
			"node-1",
			model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.05}},
			model.ServiceOperationQPS{"svcA": {http.MethodGet: 1.0}},
		))

		mockEP := &epmocks.ElectionParticipant{}
		mockEP.On("IsLeader").Return(false).Once()
		// Simulate previous leader persisting newer 0.0001 right before this node observes promotion.
		mockEP.On("IsLeader").
			Run(func(mock.Arguments) {
				require.NoError(t, sharedStore.InsertProbabilitiesAndQPS(
					"node-1",
					model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.0001}},
					model.ServiceOperationQPS{"svcA": {http.MethodGet: 1.0}},
				))
			}).
			Return(true).
			Once()

		pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "node-2", sharedStore, mockEP, metrics.NullFactory, zap.NewNop())
		require.NoError(t, err)
		pa.Start()

		time.Sleep(time.Second)
		require.NoError(t, sharedStore.InsertThroughput([]*model.Throughput{
			{
				Service:       "svcA",
				Operation:     http.MethodGet,
				Count:         1,
				Probabilities: map[string]struct{}{"0.000100": {}},
			},
		}))
		synctest.Wait()
		pa.runCalculation()

		got, err := sharedStore.GetLatestProbabilities()
		require.NoError(t, err)
		assert.InDelta(t, 0.0001, got["svcA"][http.MethodGet], 1e-9)
		mockEP.AssertExpectations(t)
	})
}

func TestPostAggregatorInsertFailurePreservesInMemoryProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockStorage := &smocks.Store{}
		mockStorage.On("GetThroughput", mock.Anything, mock.Anything).Return([]*model.Throughput{
			{
				Service:       "svcA",
				Operation:     http.MethodGet,
				Count:         2,
				Probabilities: map[string]struct{}{"0.100000": {}, "0.050000": {}},
			},
		}, nil)
		// Persisted rate remains 0.1 across ticks because Tick 1's InsertProbabilitiesAndQPS fails.
		mockStorage.On("GetLatestProbabilities").Return(model.ServiceOperationProbabilities{"svcA": {http.MethodGet: 0.1}}, nil).Times(3)
		mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.Anything, mock.Anything).Return(errors.New("write failed")).Once()

		var tick2Saved model.ServiceOperationProbabilities
		mockStorage.On("InsertProbabilitiesAndQPS", "host", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				tick2Saved = args.Get(1).(model.ServiceOperationProbabilities)
			}).
			Return(nil).
			Once()

		mockEP := &epmocks.ElectionParticipant{}
		mockEP.On("IsLeader").Return(true)

		pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "host", mockStorage, mockEP, metrics.NullFactory, zap.NewNop())
		require.NoError(t, err)
		pa.Start()

		// Tick 1: 2 QPS at 0.1 -> computes 0.05 in memory, write fails so storage keeps 0.1.
		time.Sleep(time.Second)
		synctest.Wait()
		pa.runCalculation()

		pa.mu.RLock()
		assert.InDelta(t, 0.05, pa.probabilities["svcA"][http.MethodGet], 1e-9)
		pa.mu.RUnlock()

		// Tick 2: continuing leader refreshes storage (still 0.1), must not roll back 0.05 -> scales 0.05 to 0.025.
		time.Sleep(time.Second)
		synctest.Wait()
		pa.runCalculation()

		require.InDelta(t, 0.025, tick2Saved["svcA"][http.MethodGet], 1e-9)
		mockStorage.AssertExpectations(t)
	})
}

func TestPostAggregatorElasticsearchMissingIndexVsFailures(t *testing.T) {
	throughputRaw, err := json.Marshal(esdbmodel.TimeThroughput{
		Timestamp: time.Unix(0, 0).UTC(),
		Throughput: esdbmodel.Throughput{
			Service:       "svcA",
			Operation:     http.MethodGet,
			Count:         2,
			Probabilities: map[string]struct{}{"0.100000": {}},
		},
	})
	require.NoError(t, err)

	tests := []struct {
		name           string
		indexExists    bool
		indexErr       error
		probSearchErr  error
		expectPersists bool
	}{
		{
			name:           "no matching index",
			indexExists:    false,
			expectPersists: true,
		},
		{
			name:           "existing index without probability documents",
			indexExists:    true,
			expectPersists: true,
		},
		{
			name:           "index check failure defers calculation",
			indexErr:       errors.New("index check failed"),
			expectPersists: false,
		},
		{
			name:           "search failure defers calculation",
			indexExists:    true,
			probSearchErr:  errors.New("search failed"),
			expectPersists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				indexClient := esclientmocks.NewIndexAPI(t)
				indexClient.On("IndexExists", mock.Anything, "jaeger-sampling-read").Return(tt.indexExists, tt.indexErr)

				searcher := esclientmocks.NewSearcher(t)
				searcher.On("Search", mock.Anything, mock.Anything, mock.MatchedBy(func(req esclient.SearchRequest) bool {
					return req.Query != nil
				})).Return(&esclient.SearchResponse{
					Hits: esclient.HitsResult{
						Hits: []esclient.SearchHit{{Source: json.RawMessage(throughputRaw)}},
					},
				}, nil)
				if tt.indexExists && tt.indexErr == nil {
					searcher.On("Search", mock.Anything, []string{"jaeger-sampling-read"}, mock.MatchedBy(func(req esclient.SearchRequest) bool {
						return req.Query == nil
					})).Return(&esclient.SearchResponse{}, tt.probSearchErr)
				}

				batchWriter := esclientmocks.NewBatchWriter(t)
				var written []esclient.BulkItem
				if tt.expectPersists {
					batchWriter.On("WriteBatch", mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) {
							written = args.Get(1).([]esclient.BulkItem)
						}).
						Return(nil).
						Once()
				}

				store := essamplestore.NewSamplingStore(essamplestore.Params{
					Searcher:    searcher,
					BatchWriter: batchWriter,
					IndexClient: indexClient,
					Logger:      zap.NewNop(),
					MaxDocCount: 1000,
					Lookback:    72 * time.Hour,
					Rotation:    indices.NewAliasedRotation("jaeger-sampling-write", "jaeger-sampling-read"),
				})

				mockEP := &epmocks.ElectionParticipant{}
				mockEP.On("IsLeader").Return(true)

				pa, err := newPostAggregator(newTestAdaptiveOpts(time.Second, 1.0, 0.1), "host", store, mockEP, metrics.NullFactory, zap.NewNop())
				require.NoError(t, err)
				pa.Start()

				time.Sleep(time.Second)
				synctest.Wait()
				pa.runCalculation()

				if tt.expectPersists {
					require.Len(t, written, 1)
					assert.Equal(t, "jaeger-sampling-write", written[0].Index)
					body := written[0].Body.(*esdbmodel.TimeProbabilitiesAndQPS)
					assert.InDelta(t, 0.05, body.ProbabilitiesAndQPS.Probabilities["svcA"][http.MethodGet], 1e-9)
				} else {
					batchWriter.AssertNotCalled(t, "WriteBatch", mock.Anything, mock.Anything)
					pa.mu.RLock()
					assert.Empty(t, pa.probabilities)
					pa.mu.RUnlock()
				}
			})
		})
	}
}
