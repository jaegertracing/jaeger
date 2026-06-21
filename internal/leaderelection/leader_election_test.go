// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package leaderelection

import (
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestCloseForfeitsLeaderLock(t *testing.T) {
	const (
		leaderInterval   = time.Millisecond
		followerInterval = 5 * time.Millisecond
	)

	mockLock := lmocks.NewLock(t)
	mockLock.On("Acquire", "sampling_lock", followerInterval).Return(true, nil)
	mockLock.On("Forfeit", "sampling_lock").Return(true, nil)

	logger, _ := testutils.NewLogger()
	p := NewElectionParticipant(
		mockLock, "sampling_lock", ElectionParticipantOptions{
			LeaderLeaseRefreshInterval:   leaderInterval,
			FollowerLeaseRefreshInterval: followerInterval,
			Logger:                       logger,
		},
	)

	require.NoError(t, p.Start())
	require.Eventually(t, p.IsLeader, time.Second, time.Millisecond,
		"participant should become leader")

	require.NoError(t, p.Close())
	mockLock.AssertCalled(t, "Forfeit", "sampling_lock")
}

func TestCloseDoesNotForfeitIfFollower(t *testing.T) {
	const (
		leaderInterval   = time.Millisecond
		followerInterval = 5 * time.Millisecond
	)

	mockLock := lmocks.NewLock(t)
	mockLock.On("Acquire", "sampling_lock", followerInterval).Return(false, nil)

	logger, _ := testutils.NewLogger()
	p := NewElectionParticipant(
		mockLock, "sampling_lock", ElectionParticipantOptions{
			LeaderLeaseRefreshInterval:   leaderInterval,
			FollowerLeaseRefreshInterval: followerInterval,
			Logger:                       logger,
		},
	)

	require.NoError(t, p.Start())
	// Give the loop a chance to run at least once.
	time.Sleep(10 * time.Millisecond)
	assert.False(t, p.IsLeader())

	require.NoError(t, p.Close())
	mockLock.AssertNotCalled(t, "Forfeit", "sampling_lock")
}

func TestCloseForfeitError(t *testing.T) {
	const (
		leaderInterval   = time.Millisecond
		followerInterval = 5 * time.Millisecond
	)

	mockLock := lmocks.NewLock(t)
	mockLock.On("Acquire", "sampling_lock", followerInterval).Return(true, nil)
	mockLock.On("Forfeit", "sampling_lock").Return(false, errTestLock)

	logger, logBuffer := testutils.NewLogger()
	p := NewElectionParticipant(
		mockLock, "sampling_lock", ElectionParticipantOptions{
			LeaderLeaseRefreshInterval:   leaderInterval,
			FollowerLeaseRefreshInterval: followerInterval,
			Logger:                       logger,
		},
	)

	require.NoError(t, p.Start())
	require.Eventually(t, p.IsLeader, time.Second, time.Millisecond,
		"participant should become leader")

	// Close should not return an error even if Forfeit fails.
	require.NoError(t, p.Close())
	mockLock.AssertCalled(t, "Forfeit", "sampling_lock")

	match, errMsg := testutils.LogMatcher(1, forfeitLockErrMsg, logBuffer.Lines())
	assert.True(t, match, errMsg)
}

func TestCloseForfeitNotOwner(t *testing.T) {
	const (
		leaderInterval   = time.Millisecond
		followerInterval = 5 * time.Millisecond
	)

	errNotOwner := errors.New("failed to forfeit resource lock: this host does not own the resource lock")

	mockLock := lmocks.NewLock(t)
	mockLock.On("Acquire", "sampling_lock", followerInterval).Return(true, nil)
	mockLock.On("Forfeit", "sampling_lock").Return(false, errNotOwner)

	logger, logBuffer := testutils.NewLogger()
	p := NewElectionParticipant(
		mockLock, "sampling_lock", ElectionParticipantOptions{
			LeaderLeaseRefreshInterval:   leaderInterval,
			FollowerLeaseRefreshInterval: followerInterval,
			Logger:                       logger,
		},
	)

	require.NoError(t, p.Start())
	require.Eventually(t, p.IsLeader, time.Second, time.Millisecond,
		"participant should become leader")

	require.NoError(t, p.Close())
	mockLock.AssertCalled(t, "Forfeit", "sampling_lock")

	match, errMsg := testutils.LogMatcher(1, forfeitLockErrMsg, logBuffer.Lines())
	assert.True(t, match, errMsg)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
