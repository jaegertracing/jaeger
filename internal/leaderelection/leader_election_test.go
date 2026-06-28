// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package leaderelection

import (
	"errors"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	lmocks "github.com/jaegertracing/jaeger/internal/distributedlock/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var errTestLock = errors.New("lock error")

var _ io.Closer = new(DistributedElectionParticipant)

func TestAcquireLock(t *testing.T) {
	const (
		leaderInterval   = time.Millisecond
		followerInterval = 5 * time.Millisecond
	)
	tests := []struct {
		isLeader         bool
		acquiredLock     bool
		err              error
		expectedInterval time.Duration
		expectedError    bool
	}{
		{isLeader: true, acquiredLock: true, err: nil, expectedInterval: leaderInterval, expectedError: false},
		{isLeader: true, acquiredLock: false, err: errTestLock, expectedInterval: leaderInterval, expectedError: true},
		{isLeader: true, acquiredLock: false, err: nil, expectedInterval: followerInterval, expectedError: false},
		{isLeader: false, acquiredLock: false, err: nil, expectedInterval: followerInterval, expectedError: false},
		{isLeader: false, acquiredLock: false, err: errTestLock, expectedInterval: followerInterval, expectedError: true},
		{isLeader: false, acquiredLock: true, err: nil, expectedInterval: leaderInterval, expectedError: false},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			logger, logBuffer := testutils.NewLogger()
			mockLock := &lmocks.Lock{}
			mockLock.On("Acquire", "sampling_lock", followerInterval).Return(test.acquiredLock, test.err)

			p := &DistributedElectionParticipant{
				ElectionParticipantOptions: ElectionParticipantOptions{
					LeaderLeaseRefreshInterval:   leaderInterval,
					FollowerLeaseRefreshInterval: followerInterval,
					Logger:                       logger,
				},
				lock:         mockLock,
				resourceName: "sampling_lock",
			}

			p.setLeader(test.isLeader)
			assert.Equal(t, test.expectedInterval, p.acquireLock())
			match, errMsg := testutils.LogMatcher(1, acquireLockErrMsg, logBuffer.Lines())
			assert.Equal(t, test.expectedError, match, errMsg)
		})
	}
}

func TestRunAcquireLockLoopFollowerOnly(t *testing.T) {
	logger, logBuffer := testutils.NewLogger()
	mockLock := &lmocks.Lock{}
	mockLock.On("Acquire", "sampling_lock", time.Duration(5*time.Millisecond)).Return(false, errTestLock)

	p := NewElectionParticipant(
		mockLock, "sampling_lock", ElectionParticipantOptions{
			LeaderLeaseRefreshInterval:   time.Millisecond,
			FollowerLeaseRefreshInterval: 5 * time.Millisecond,
			Logger:                       logger,
		},
	)

	defer func() {
		require.NoError(t, p.Close())
	}()
	go p.Start()

	expectedErrorMsg := "Failed to acquire lock"
	for range 1000 {
		// match logs specific to acquireLockErrMsg.
		if match, _ := testutils.LogMatcher(2, expectedErrorMsg, logBuffer.Lines()); match {
			break
		}
		time.Sleep(time.Millisecond)
	}
	match, errMsg := testutils.LogMatcher(2, expectedErrorMsg, logBuffer.Lines())
	assert.True(t, match, errMsg)
	assert.False(t, p.IsLeader())
}

type closeTestSetup struct {
	mockLock  *lmocks.Lock
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	p         *DistributedElectionParticipant
}

func newCloseTestSetup(t *testing.T) *closeTestSetup {
	t.Helper()
	const (
		leaderInterval   = time.Millisecond
		followerInterval = 5 * time.Millisecond
	)
	mockLock := lmocks.NewLock(t)
	logger, logBuffer := testutils.NewLogger()
	p := NewElectionParticipant(
		mockLock, "sampling_lock", ElectionParticipantOptions{
			LeaderLeaseRefreshInterval:   leaderInterval,
			FollowerLeaseRefreshInterval: followerInterval,
			Logger:                       logger,
		},
	)
	return &closeTestSetup{
		mockLock:  mockLock,
		logger:    logger,
		logBuffer: logBuffer,
		p:         p,
	}
}

func TestCloseForfeitsLeaderLock(t *testing.T) {
	s := newCloseTestSetup(t)
	s.mockLock.On("Acquire", "sampling_lock", 5*time.Millisecond).Return(true, nil)
	s.mockLock.On("Forfeit", "sampling_lock").Return(true, nil)

	require.NoError(t, s.p.Start())
	require.Eventually(t, s.p.IsLeader, time.Second, time.Millisecond,
		"participant should become leader")

	require.NoError(t, s.p.Close())
	s.mockLock.AssertCalled(t, "Forfeit", "sampling_lock")
}

func TestCloseDoesNotForfeitIfFollower(t *testing.T) {
	s := newCloseTestSetup(t)
	s.mockLock.On("Acquire", "sampling_lock", 5*time.Millisecond).Return(false, nil)

	require.NoError(t, s.p.Start())
	require.Eventually(t, func() bool {
		return !s.p.IsLeader()
	}, time.Second, time.Millisecond, "participant should remain follower")

	require.NoError(t, s.p.Close())
	s.mockLock.AssertNotCalled(t, "Forfeit", "sampling_lock")
}

func TestCloseForfeitError(t *testing.T) {
	s := newCloseTestSetup(t)
	s.mockLock.On("Acquire", "sampling_lock", 5*time.Millisecond).Return(true, nil)
	s.mockLock.On("Forfeit", "sampling_lock").Return(false, errTestLock)

	require.NoError(t, s.p.Start())
	require.Eventually(t, s.p.IsLeader, time.Second, time.Millisecond,
		"participant should become leader")

	// Close should not return an error even if Forfeit fails.
	require.NoError(t, s.p.Close())
	s.mockLock.AssertCalled(t, "Forfeit", "sampling_lock")

	match, errMsg := testutils.LogMatcher(1, forfeitLockErrMsg, s.logBuffer.Lines())
	assert.True(t, match, errMsg)
}

func TestCloseForfeitNotForfeited(t *testing.T) {
	s := newCloseTestSetup(t)
	s.mockLock.On("Acquire", "sampling_lock", 5*time.Millisecond).Return(true, nil)
	s.mockLock.On("Forfeit", "sampling_lock").Return(false, nil)

	require.NoError(t, s.p.Start())
	require.Eventually(t, s.p.IsLeader, time.Second, time.Millisecond,
		"participant should become leader")

	require.NoError(t, s.p.Close())
	s.mockLock.AssertCalled(t, "Forfeit", "sampling_lock")

	match, errMsg := testutils.LogMatcher(1, forfeitLockWarnMsg, s.logBuffer.Lines())
	assert.True(t, match, errMsg)
}

func TestCloseIsIdempotent(t *testing.T) {
	s := newCloseTestSetup(t)
	s.mockLock.On("Acquire", "sampling_lock", 5*time.Millisecond).Return(true, nil)
	s.mockLock.On("Forfeit", "sampling_lock").Return(true, nil)

	require.NoError(t, s.p.Start())
	require.Eventually(t, s.p.IsLeader, time.Second, time.Millisecond,
		"participant should become leader")

	require.NoError(t, s.p.Close())
	// Second close must not panic.
	require.NoError(t, s.p.Close())
}

func TestCloseConcurrentIsIdempotent(t *testing.T) {
	s := newCloseTestSetup(t)

	s.mockLock.On("Acquire", "sampling_lock", 5*time.Millisecond).
		Return(true, nil)

	s.mockLock.On("Forfeit", "sampling_lock").
		Return(true, nil).
		Once()

	require.NoError(t, s.p.Start())

	require.Eventually(
		t, s.p.IsLeader,
		time.Second,
		time.Millisecond,
		"participant should become leader",
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		assert.NoError(t, s.p.Close())
	}()

	go func() {
		defer wg.Done()
		assert.NoError(t, s.p.Close())
	}()

	wg.Wait()

	s.mockLock.AssertNumberOfCalls(t, "Forfeit", 1)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
